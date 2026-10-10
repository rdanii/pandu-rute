package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pandu-rute/backend/internal/tripplan"
	"pandu-rute/backend/internal/tripplan/tripplantest"
)

const validBody = `{
  "origin": {"lat": -6.2, "lon": 106.8, "label": "Rumah"},
  "destination": {"lat": -6.1, "lon": 106.81},
  "departAt": "2026-10-12T08:00:00+07:00"
}`

var fixedNow = time.Date(2026, 10, 10, 1, 30, 0, 0, time.UTC) // 08:30 WIB

type testEnv struct {
	planner *tripplantest.FakePlanner
	logs    *bytes.Buffer
	handler http.Handler
}

func newEnv(p *tripplantest.FakePlanner) *testEnv {
	logs := &bytes.Buffer{}
	d := Dependencies{
		Logger:      slog.New(slog.NewTextHandler(logs, nil)),
		Now:         func() time.Time { return fixedNow },
		PlanTimeout: time.Second,
	}
	if p != nil {
		d.Planner = p
	}
	return &testEnv{planner: p, logs: logs, handler: NewRouterWith(d)}
}

func (e *testEnv) post(body, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/trip-plans", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v; body=%s", err, rec.Body)
	}
	if env.Error.RequestID == "" || env.Error.RequestID != rec.Header().Get("X-Request-Id") {
		t.Errorf("requestId %q does not match header %q", env.Error.RequestID, rec.Header().Get("X-Request-Id"))
	}
	if env.Error.Message == "" {
		t.Error("empty error message")
	}
	return env.Error
}

func samplePlanner() *tripplantest.FakePlanner {
	depart := time.Date(2026, 10, 12, 8, 6, 0, 0, time.UTC)
	return &tripplantest.FakePlanner{Result: tripplantest.SamplePlan(depart)}
}

func TestTripPlanSuccess(t *testing.T) {
	env := newEnv(samplePlanner())
	rec := env.post(validBody, "application/json; charset=utf-8")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Error("missing X-Request-Id")
	}
	var resp tripPlanResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Origin.Label != "Rumah" || resp.Destination.Label != "" {
		t.Errorf("labels not echoed as given: %+v %+v", resp.Origin, resp.Destination)
	}
	if resp.RequestedDepartAt != "2026-10-12T08:00:00+07:00" {
		t.Errorf("requestedDepartAt = %q", resp.RequestedDepartAt)
	}
	if len(resp.Itineraries) != 1 {
		t.Fatalf("itineraries = %d", len(resp.Itineraries))
	}
	it := resp.Itineraries[0]
	if it.Departure != "2026-10-12T15:06:00+07:00" || it.TimeQuality != "frequency_estimate" ||
		it.DurationSeconds != 1500 || len(it.Legs) != 2 || it.Legs[0].Line.ID != "test:line:1" {
		t.Errorf("itinerary = %+v", it)
	}
	if len(resp.Attributions) != 1 || resp.Attributions[0].ID != "test-source" {
		t.Errorf("attributions = %+v", resp.Attributions)
	}
	if resp.DataVersion != nil {
		t.Errorf("dataVersion = %+v, want null when planner gives none", resp.DataVersion)
	}
	if !strings.Contains(rec.Body.String(), `"dataVersion":null`) {
		t.Error("dataVersion should be serialised as null")
	}

	q := env.planner.Queries()
	if len(q) != 1 {
		t.Fatalf("planner calls = %d", len(q))
	}
	if q[0].MaxItineraries != tripplan.DefaultMaxItineraries {
		t.Errorf("maxItineraries = %d, want default", q[0].MaxItineraries)
	}
	if q[0].Origin.Coordinate != (tripplan.Coordinate{Lat: -6.2, Lon: 106.8}) || q[0].Origin.Label != "Rumah" {
		t.Errorf("origin = %+v", q[0].Origin)
	}
}

func TestTripPlanDefaultsDepartAtToNowInWIB(t *testing.T) {
	env := newEnv(samplePlanner())
	rec := env.post(`{"origin":{"lat":-6.2,"lon":106.8},"destination":{"lat":-6.1,"lon":106.81},"maxItineraries":5}`, "application/json")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	q := env.planner.Queries()[0]
	if !q.DepartAt.Equal(fixedNow) || q.DepartAt.Format(time.RFC3339) != "2026-10-10T08:30:00+07:00" {
		t.Errorf("departAt = %v", q.DepartAt)
	}
	if q.MaxItineraries != 5 {
		t.Errorf("maxItineraries = %d", q.MaxItineraries)
	}
}

func TestTripPlanInvalidInput(t *testing.T) {
	cases := []struct {
		name, body, field, reason string
	}{
		{"empty body", ``, "body", "required"},
		{"malformed json", `{"origin":`, "body", "malformed_json"},
		{"not an object", `[1,2]`, "body", "invalid_type"},
		{"trailing value", validBody + ` {}`, "body", "trailing_data"},
		{"unknown top-level field", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"query":"Blok M"}`, "query", "unknown_field"},
		{"unknown nested field", `{"origin":{"lat":1,"lon":1,"alt":5},"destination":{"lat":1,"lon":2}}`, "alt", "unknown_field"},
		{"missing origin", `{"destination":{"lat":1,"lon":2}}`, "origin", "required"},
		{"null origin", `{"origin":null,"destination":{"lat":1,"lon":2}}`, "origin", "required"},
		{"missing lat", `{"origin":{"lon":1},"destination":{"lat":1,"lon":2}}`, "origin.lat", "required"},
		{"missing destination lon", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1}}`, "destination.lon", "required"},
		{"lat as string", `{"origin":{"lat":"-6.2","lon":1},"destination":{"lat":1,"lon":2}}`, "origin.lat", "invalid_type"},
		{"lat too large", `{"origin":{"lat":90.0001,"lon":1},"destination":{"lat":1,"lon":2}}`, "origin.lat", "out_of_range"},
		{"lon too small", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":-180.5}}`, "destination.lon", "out_of_range"},
		{"departAt without offset", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"departAt":"2026-10-12T08:00:00"}`, "departAt", "invalid_format"},
		{"departAt date only", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"departAt":"2026-10-12"}`, "departAt", "invalid_format"},
		{"departAt wrong type", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"departAt":1760000000}`, "departAt", "invalid_type"},
		{"maxItineraries zero", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"maxItineraries":0}`, "maxItineraries", "out_of_range"},
		{"maxItineraries six", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"maxItineraries":6}`, "maxItineraries", "out_of_range"},
		{"maxItineraries fraction", `{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"maxItineraries":2.5}`, "maxItineraries", "invalid_type"},
		{"label too long", `{"origin":{"lat":1,"lon":1,"label":"` + strings.Repeat("a", 101) + `"},"destination":{"lat":1,"lon":2}}`, "origin.label", "too_long"},
		{"label with control char", `{"origin":{"lat":1,"lon":1,"label":"a\u0007b"},"destination":{"lat":1,"lon":2}}`, "origin.label", "invalid_format"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newEnv(samplePlanner())
			rec := env.post(c.body, "application/json")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body)
			}
			e := decodeError(t, rec)
			if e.Code != CodeInvalidInput {
				t.Errorf("code = %q", e.Code)
			}
			found := false
			for _, d := range e.Details {
				found = found || (d.Field == c.field && d.Reason == c.reason)
			}
			if !found {
				t.Errorf("details = %+v, want {%s %s}", e.Details, c.field, c.reason)
			}
			if n := len(env.planner.Queries()); n != 0 {
				t.Errorf("planner called %d times on invalid input", n)
			}
		})
	}
}

func TestTripPlanAcceptsUTCAndOffsetDepartAt(t *testing.T) {
	for _, v := range []string{"2026-10-12T01:00:00Z", "2026-10-12T08:00:00.5+07:00"} {
		env := newEnv(samplePlanner())
		rec := env.post(`{"origin":{"lat":1,"lon":1},"destination":{"lat":1,"lon":2},"departAt":"`+v+`"}`, "application/json")
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body=%s", v, rec.Code, rec.Body)
		}
	}
}

func TestTripPlanRequiresJSONContentType(t *testing.T) {
	for _, ct := range []string{"", "text/plain", "application/x-www-form-urlencoded", "application/json; charset=latin1", "application/json;;"} {
		env := newEnv(samplePlanner())
		rec := env.post(validBody, ct)
		if rec.Code != http.StatusUnsupportedMediaType {
			t.Errorf("%q: status = %d, want 415", ct, rec.Code)
			continue
		}
		if e := decodeError(t, rec); e.Code != CodeUnsupportedMediaType {
			t.Errorf("%q: code = %q", ct, e.Code)
		}
	}
}

func TestTripPlanBodyLimit(t *testing.T) {
	pad := `,"origin":{"lat":1,"lon":1,"label":"` + strings.Repeat("x", 90) + `"}`
	big := `{"destination":{"lat":1,"lon":2}` + strings.Repeat(" ", maxTripPlanBody) + pad + `}`

	t.Run("declared length", func(t *testing.T) {
		env := newEnv(samplePlanner())
		rec := env.post(big, "application/json")
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413", rec.Code)
		}
		if e := decodeError(t, rec); e.Code != CodePayloadTooLarge {
			t.Errorf("code = %q", e.Code)
		}
	})
	t.Run("unknown length", func(t *testing.T) {
		env := newEnv(samplePlanner())
		req := httptest.NewRequest(http.MethodPost, "/v1/trip-plans", io.MultiReader(strings.NewReader(big)))
		req.ContentLength = -1
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body)
		}
	})
	t.Run("exactly at limit is accepted", func(t *testing.T) {
		body := validBody + strings.Repeat(" ", maxTripPlanBody-len(validBody))
		env := newEnv(samplePlanner())
		if rec := env.post(body, "application/json"); rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
		}
	})
}

func TestTripPlanErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		field  string
	}{
		{"no route", tripplan.ErrNoRoute, 422, CodeNoRoute, ""},
		{"out of coverage", &tripplan.OutOfCoverageError{Side: tripplan.SideOrigin}, 422, CodeOutOfCoverage, "origin"},
		{"out of coverage unknown side", &tripplan.OutOfCoverageError{}, 422, CodeOutOfCoverage, ""},
		{"not routable", &tripplan.LocationNotRoutableError{Side: tripplan.SideDestination}, 422, CodeLocationNotRoutable, "destination"},
		{"unavailable", tripplan.ErrRoutingUnavailable, 503, CodeRoutingUnavailable, ""},
		{"timeout", tripplan.ErrRoutingTimeout, 504, CodeRoutingTimeout, ""},
		{"raw deadline", context.DeadlineExceeded, 504, CodeRoutingTimeout, ""},
		{"upstream invalid", fmt.Errorf("%w: SECRET upstream body", tripplan.ErrUpstreamInvalid), 502, CodeUpstreamInvalidResponse, ""},
		{"unexpected", errors.New("SECRET graphql error text"), 500, CodeInternal, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := newEnv(&tripplantest.FakePlanner{Err: c.err})
			rec := env.post(validBody, "application/json")
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			e := decodeError(t, rec)
			if e.Code != c.code {
				t.Errorf("code = %q, want %q", e.Code, c.code)
			}
			if c.field != "" && (len(e.Details) != 1 || e.Details[0].Field != c.field) {
				t.Errorf("details = %+v, want field %q", e.Details, c.field)
			}
			if c.field == "" && len(e.Details) != 0 {
				t.Errorf("details = %+v, want none", e.Details)
			}
			for _, s := range []string{rec.Body.String(), env.logs.String()} {
				if strings.Contains(s, "SECRET") {
					t.Errorf("upstream error text leaked: %s", s)
				}
			}
		})
	}
}

func TestTripPlanWithoutPlannerIsUnavailable(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/trip-plans", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")
	NewRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != CodeRoutingUnavailable {
		t.Errorf("code = %q", e.Code)
	}
}

func TestTripPlanPlannerTimeout(t *testing.T) {
	logs := &bytes.Buffer{}
	h := NewRouterWith(Dependencies{
		Planner:     &tripplantest.FakePlanner{Block: true},
		Logger:      slog.New(slog.NewTextHandler(logs, nil)),
		PlanTimeout: 30 * time.Millisecond,
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/trip-plans", strings.NewReader(validBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != CodeRoutingTimeout {
		t.Errorf("code = %q", e.Code)
	}
}

func TestTripPlanRejectsPlanWithoutAttribution(t *testing.T) {
	p := samplePlanner()
	p.Result.Attributions = nil
	rec := newEnv(p).post(validBody, "application/json")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if e := decodeError(t, rec); e.Code != CodeUpstreamInvalidResponse {
		t.Errorf("code = %q", e.Code)
	}
}

func TestTripPlanLogsAreCoarse(t *testing.T) {
	env := newEnv(samplePlanner())
	env.post(validBody, "application/json")
	env.post(`{"origin":{"lat":-6.2,"lon":106.8,"label":"Rumah"}}`, "application/json")
	logs := env.logs.String()
	for _, s := range []string{"-6.2", "106.8", "Rumah"} {
		if strings.Contains(logs, s) {
			t.Errorf("log contains %q: %s", s, logs)
		}
	}
	if !strings.Contains(logs, "code=INVALID_INPUT") || !strings.Contains(logs, "status=200") {
		t.Errorf("expected outcome fields in logs: %s", logs)
	}
}

func TestTripPlanRejectsOtherMethods(t *testing.T) {
	rec := httptest.NewRecorder()
	NewRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/trip-plans", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestRequestIDsAreUnique(t *testing.T) {
	env := newEnv(nil)
	a := env.post(validBody, "application/json").Header().Get("X-Request-Id")
	b := env.post(validBody, "application/json").Header().Get("X-Request-Id")
	if a == "" || a == b || !strings.HasPrefix(a, "req_") {
		t.Errorf("request IDs %q, %q", a, b)
	}
}
