package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const tripBody = `{"origin":{"lat":-6.2,"lon":106.8},"destination":{"lat":-6.1205,"lon":106.8105},"departAt":"2026-10-12T08:00:00+07:00"}`

// Synthetic OTP answer (made-up ids and names).
const fakeOTPResponse = `{"data":{"planConnection":{"routingErrors":[],"edges":[{"node":{
 "start":"2026-10-12T08:06:00+07:00","end":"2026-10-12T08:30:00+07:00","duration":1440,"numberOfTransfers":0,"walkDistance":0,
 "legs":[{"mode":"BUS","transitLeg":true,"duration":1440,"distance":8000,"headsign":"Uji B",
  "start":{"scheduledTime":"2026-10-12T08:06:00+07:00"},"end":{"scheduledTime":"2026-10-12T08:30:00+07:00"},
  "from":{"name":"Halte Uji A","lat":-6.2,"lon":106.8,"stop":{"gtfsId":"1:S1"}},
  "to":{"name":"Halte Uji B","lat":-6.1205,"lon":106.8105,"stop":{"gtfsId":"1:S2"}},
  "route":{"gtfsId":"1:R1","shortName":"U1","longName":"Uji A - Uji B","mode":"BUS"},"trip":{"gtfsId":"1:R1-T1"}}]}}]}}}`

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestServerRoutesThroughOTP(t *testing.T) {
	otpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/otp/gtfs/v1" {
			http.NotFound(w, r)
			return
		}
		_, _ = io.WriteString(w, fakeOTPResponse)
	}))
	defer otpSrv.Close()

	srv, _, err := newServer(env(map[string]string{"OTP_BASE_URL": otpSrv.URL}))
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, srv.Handler, http.MethodPost, "/v1/trip-plans", tripBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Itineraries []struct {
			TimeQuality string `json:"timeQuality"`
			Legs        []struct {
				Line struct{ ID string } `json:"line"`
			} `json:"legs"`
		} `json:"itineraries"`
		Attributions []struct{ ID, License string } `json:"attributions"`
		DataVersion  *json.RawMessage               `json:"dataVersion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Itineraries) != 1 || resp.Itineraries[0].Legs[0].Line.ID != "tj:line:R1" ||
		resp.Itineraries[0].TimeQuality != "frequency_estimate" {
		t.Errorf("itineraries = %+v", resp.Itineraries)
	}
	if len(resp.Attributions) != 2 || resp.Attributions[0].ID != "transjakarta-gtfs" || resp.Attributions[1].ID != "openstreetmap" {
		t.Errorf("attributions = %+v", resp.Attributions)
	}
	if resp.DataVersion != nil {
		t.Errorf("dataVersion should be null, got %s", *resp.DataVersion)
	}
	for _, s := range []string{"planConnection", "gtfsId", "scheduledTime", "transitLeg", "1:R1"} {
		if strings.Contains(rec.Body.String(), s) {
			t.Errorf("OTP-specific %q leaked into response", s)
		}
	}
}

func TestServerSurvivesUnavailableOTP(t *testing.T) {
	dead := httptest.NewServer(http.NotFoundHandler())
	url := dead.URL
	dead.Close()

	srv, _, err := newServer(env(map[string]string{"OTP_BASE_URL": url}))
	if err != nil {
		t.Fatalf("newServer must not fail when OTP is down: %v", err)
	}
	rec := do(t, srv.Handler, http.MethodPost, "/v1/trip-plans", tripBody)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"ROUTING_UNAVAILABLE"`) {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "refused") || strings.Contains(rec.Body.String(), "127.0.0.1") {
		t.Errorf("transport detail leaked: %s", rec.Body)
	}
	if h := do(t, srv.Handler, http.MethodGet, "/healthz", ""); h.Code != http.StatusOK || strings.TrimSpace(h.Body.String()) != `{"status":"ok"}` {
		t.Errorf("healthz = %d %s", h.Code, h.Body)
	}
}

func TestServerConfig(t *testing.T) {
	srv, planner, err := newServer(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if srv.Addr != ":8080" || planner.Endpoint() != "http://localhost:8082/otp/gtfs/v1" {
		t.Errorf("defaults: addr=%q endpoint=%q", srv.Addr, planner.Endpoint())
	}
	if srv.WriteTimeout != 10*time.Second || srv.ReadTimeout != 10*time.Second || srv.ReadHeaderTimeout != 5*time.Second {
		t.Errorf("server timeouts changed: %+v", srv)
	}
	if _, _, err := newServer(env(map[string]string{"OTP_BASE_URL": "not a url"})); err == nil {
		t.Error("invalid OTP_BASE_URL accepted")
	}
	if _, _, err := newServer(env(map[string]string{"OTP_BASE_URL": "http://u:p@otp.test"})); err == nil {
		t.Error("credentials in OTP_BASE_URL accepted")
	}
}
