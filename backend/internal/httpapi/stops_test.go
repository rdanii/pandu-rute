package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pandu-rute/backend/internal/stops"
	"pandu-rute/backend/internal/tripplan"
	"pandu-rute/backend/internal/tripplan/tripplantest"
)

// Synthetic stops; names, ids and coordinates are made up.
const stopsCSV = `stop_id,stop_name,stop_lat,stop_lon
A1,Halte Uji Alpha,-6.201,106.801
B2,Halte Uji Beta,-6.202,106.802
C3,Terminal Contoh,-6.203,106.803
`

var testStopAttribution = tripplan.Attribution{ID: "test-gtfs", Text: "Data halte uji", License: "TEST"}

func mustIndex(t *testing.T, csv string) *stops.Index {
	t.Helper()
	idx, err := stops.Parse(strings.NewReader(csv), "tj")
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

type stopsEnv struct {
	handler http.Handler
	logs    *bytes.Buffer
	planner *tripplantest.FakePlanner
}

func newStopsEnv(idx *stops.Index, attrs []tripplan.Attribution) *stopsEnv {
	logs := &bytes.Buffer{}
	p := samplePlanner()
	return &stopsEnv{
		logs:    logs,
		planner: p,
		handler: NewRouterWith(Dependencies{
			Planner:          p,
			Stops:            idx,
			StopAttributions: attrs,
			Logger:           slog.New(slog.NewTextHandler(logs, nil)),
			Now:              func() time.Time { return fixedNow },
		}),
	}
}

func (e *stopsEnv) get(rawQuery string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/stops?"+rawQuery, nil))
	return rec
}

func decodeStops(t *testing.T, rec *httptest.ResponseRecorder) stopSearchResponse {
	t.Helper()
	var resp stopSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body)
	}
	return resp
}

func TestStopSearchSuccess(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution})
	rec := env.get("q=halte+uji")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("X-Request-Id") == "" || rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", rec.Header())
	}
	resp := decodeStops(t, rec)
	// Both are prefix matches; the shorter normalised name ranks first.
	want := []stopResponse{
		{ID: "tj:stop:B2", Name: "Halte Uji Beta", Lat: -6.202, Lon: 106.802},
		{ID: "tj:stop:A1", Name: "Halte Uji Alpha", Lat: -6.201, Lon: 106.801},
	}
	if fmt.Sprint(resp.Stops) != fmt.Sprint(want) {
		t.Errorf("stops = %+v", resp.Stops)
	}
	if len(resp.Attributions) != 1 || resp.Attributions[0].ID != "test-gtfs" {
		t.Errorf("attributions = %+v", resp.Attributions)
	}
	if resp.DataVersion != nil || !strings.Contains(rec.Body.String(), `"dataVersion":null`) {
		t.Errorf("index without a file hash must report dataVersion null, got %s", rec.Body)
	}
}

func TestStopSearchReportsFileHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stops.txt")
	if err := os.WriteFile(path, []byte(stopsCSV), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := stops.LoadFile(path, "tj")
	if err != nil {
		t.Fatal(err)
	}
	resp := decodeStops(t, newStopsEnv(idx, []tripplan.Attribution{testStopAttribution}).get("q=terminal"))
	if resp.DataVersion == nil || len(resp.DataVersion.Sources) != 1 ||
		resp.DataVersion.Sources[0].ID != "gtfs-stops" || resp.DataVersion.Sources[0].SHA256 != idx.Version() {
		t.Errorf("dataVersion = %+v, want sha %s", resp.DataVersion, idx.Version())
	}
}

func TestStopSearchEmptyResult(t *testing.T) {
	rec := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution}).get("q=tidakada")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stops":[]`) {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	if resp := decodeStops(t, rec); len(resp.Attributions) != 1 {
		t.Errorf("attribution missing on empty result")
	}
}

func TestStopSearchLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("stop_id,stop_name,stop_lat,stop_lon\n")
	for i := 0; i < 25; i++ {
		fmt.Fprintf(&b, "S%02d,Halte Banyak %02d,-6.2,106.8\n", i, i)
	}
	env := newStopsEnv(mustIndex(t, b.String()), []tripplan.Attribution{testStopAttribution})
	for q, want := range map[string]int{"q=banyak": 10, "q=banyak&limit=1": 1, "q=banyak&limit=20": 20} {
		if got := len(decodeStops(t, env.get(q)).Stops); got != want {
			t.Errorf("%s: %d results, want %d", q, got, want)
		}
	}
}

func TestStopSearchValidation(t *testing.T) {
	cases := []struct{ name, query, field, reason string }{
		{"missing q", "", "q", "required"},
		{"blank q", "q=+++", "q", "required"},
		{"one character", "q=a", "q", "too_short"},
		{"one character after trim", "q=++a++", "q", "too_short"},
		{"101 characters", "q=" + strings.Repeat("a", 101), "q", "too_long"},
		{"control character", "q=ab%07cd", "q", "invalid_format"},
		{"repeated q", "q=ab&q=cd", "q", "invalid_format"},
		{"limit zero", "q=halte&limit=0", "limit", "out_of_range"},
		{"limit 21", "q=halte&limit=21", "limit", "out_of_range"},
		{"limit text", "q=halte&limit=abc", "limit", "invalid_format"},
		{"limit fraction", "q=halte&limit=2.5", "limit", "invalid_format"},
		{"repeated limit", "q=halte&limit=1&limit=2", "limit", "invalid_format"},
		{"unknown parameter", "q=halte&lat=1", "lat", "unknown_field"},
		{"malformed escape", "q=%zz", "query", "invalid_format"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution}).get(c.query)
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
		})
	}

	// 100 characters after trimming is the upper bound and is accepted.
	ok := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution}).get("q=++" + strings.Repeat("a", 100) + "++")
	if ok.Code != http.StatusOK {
		t.Errorf("100 characters: status = %d", ok.Code)
	}
}

func TestStopSearchUnavailable(t *testing.T) {
	for name, env := range map[string]*stopsEnv{
		"no index":        newStopsEnv(nil, []tripplan.Attribution{testStopAttribution}),
		"no attributions": newStopsEnv(mustIndex(t, stopsCSV), nil),
	} {
		t.Run(name, func(t *testing.T) {
			rec := env.get("q=halte")
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", rec.Code)
			}
			if e := decodeError(t, rec); e.Code != CodeStopIndexUnavailable || len(e.Details) != 0 {
				t.Errorf("error = %+v", e)
			}
			// Liveness does not depend on stop data.
			h := httptest.NewRecorder()
			env.handler.ServeHTTP(h, httptest.NewRequest(http.MethodGet, "/healthz", nil))
			if h.Code != http.StatusOK {
				t.Errorf("healthz = %d", h.Code)
			}
		})
	}
	// Input is validated before availability is checked.
	if rec := newStopsEnv(nil, nil).get("q=a"); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid input with no index: status = %d, want 400", rec.Code)
	}
}

func TestStopSearchLogsOnlyQueryLength(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution})
	env.get("q=Terminal+Contoh")
	env.get("q=x")
	logs := env.logs.String()
	for _, s := range []string{"Terminal", "Contoh", "terminal"} {
		if strings.Contains(logs, s) {
			t.Errorf("raw query leaked into logs: %s", logs)
		}
	}
	if !strings.Contains(logs, "query_len=15") || !strings.Contains(logs, "results=1") || !strings.Contains(logs, "code=INVALID_INPUT") {
		t.Errorf("expected request id, length and count in logs: %s", logs)
	}
}

func TestStopSearchRejectsOtherMethods(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution})
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/stops?q=halte", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

// A stop from the search can be used directly as a trip-plan endpoint.
func TestStopSearchResultFeedsTripPlan(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution})
	found := decodeStops(t, env.get("q=terminal+contoh")).Stops
	dest := decodeStops(t, env.get("q=halte+uji+alpha")).Stops
	if len(found) != 1 || len(dest) != 1 {
		t.Fatalf("search results: %+v / %+v", found, dest)
	}
	origin := found[0]

	body, _ := json.Marshal(map[string]any{
		"origin":      map[string]any{"lat": origin.Lat, "lon": origin.Lon, "label": origin.Name},
		"destination": map[string]any{"lat": dest[0].Lat, "lon": dest[0].Lon, "label": dest[0].Name},
		"departAt":    "2026-10-12T08:00:00+07:00",
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/trip-plans", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("trip plan status = %d, body=%s", rec.Code, rec.Body)
	}

	q := env.planner.Queries()
	if len(q) != 1 {
		t.Fatalf("planner calls = %d", len(q))
	}
	if q[0].Origin.Coordinate != (tripplan.Coordinate{Lat: origin.Lat, Lon: origin.Lon}) ||
		q[0].Destination.Coordinate != (tripplan.Coordinate{Lat: dest[0].Lat, Lon: dest[0].Lon}) {
		t.Errorf("planner got %+v / %+v", q[0].Origin, q[0].Destination)
	}
	if q[0].Origin.Label != "Terminal Contoh" {
		t.Errorf("label = %q", q[0].Origin.Label)
	}
	if !strings.HasPrefix(origin.ID, "tj:stop:") {
		t.Errorf("stop id %q does not use the trip-plan stop id format", origin.ID)
	}
}
