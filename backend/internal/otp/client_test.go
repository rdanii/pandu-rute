package otp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

// Synthetic fixtures shaped like OTP 2.10 planConnection responses observed
// locally; ids, names and times are made up.
const okResponse = `{"data":{"planConnection":{"routingErrors":[],"edges":[{"node":{
 "start":"2026-10-12T08:00:00+07:00","end":"2026-10-12T08:40:00+07:00","duration":2400,
 "numberOfTransfers":0,"walkDistance":250.4,
 "legs":[
  {"mode":"WALK","transitLeg":false,"duration":300.0,"distance":200.2,"headsign":null,
   "start":{"scheduledTime":"2026-10-12T08:00:00+07:00"},"end":{"scheduledTime":"2026-10-12T08:05:00+07:00"},
   "from":{"name":"Origin","lat":-6.2,"lon":106.8,"stop":null},
   "to":{"name":"Halte Uji A","lat":-6.201,"lon":106.801,"stop":{"gtfsId":"1:S1"}},
   "route":null,"trip":null},
  {"mode":"BUS","transitLeg":true,"duration":1980.0,"distance":9000.0,"headsign":"Uji B",
   "start":{"scheduledTime":"2026-10-12T08:06:00+07:00"},"end":{"scheduledTime":"2026-10-12T08:39+07:00"},
   "from":{"name":"Halte Uji A","lat":-6.201,"lon":106.801,"stop":{"gtfsId":"1:S1"}},
   "to":{"name":"Halte Uji B","lat":-6.12,"lon":106.81,"stop":{"gtfsId":"1:S2"}},
   "route":{"gtfsId":"1:R1","shortName":"U1","longName":"Uji A - Uji B","mode":"BUS"},"trip":{"gtfsId":"1:R1-T1"}},
  {"mode":"WALK","transitLeg":false,"duration":60.0,"distance":50.2,"headsign":null,
   "start":{"scheduledTime":"2026-10-12T08:39:00+07:00"},"end":{"scheduledTime":"2026-10-12T08:40:00+07:00"},
   "from":{"name":"Halte Uji B","lat":-6.12,"lon":106.81,"stop":{"gtfsId":"1:S2"}},
   "to":{"name":"Destination","lat":-6.1205,"lon":106.8105,"stop":null},
   "route":null,"trip":null}
 ]}}]}}}`

var depart = time.Date(2026, 10, 12, 8, 0, 0, 0, time.FixedZone("WIB", 7*3600))

func query() tripplan.Query {
	return tripplan.Query{
		Origin:         tripplan.Location{Coordinate: tripplan.Coordinate{Lat: -6.2, Lon: 106.8}, Label: "Rumah"},
		Destination:    tripplan.Location{Coordinate: tripplan.Coordinate{Lat: -6.1205, Lon: 106.8105}},
		DepartAt:       depart,
		MaxItineraries: 3,
	}
}

func testAttributions() []tripplan.Attribution {
	return []tripplan.Attribution{{ID: "test", Text: "Data uji"}}
}

func newTestClient(t *testing.T, url string, mutate ...func(*Config)) *Client {
	t.Helper()
	cfg := Config{BaseURL: url, Attributions: testAttributions(), Timeout: time.Second}
	for _, m := range mutate {
		m(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// serve returns a server that answers every request with status and body.
func serve(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func routingErrorBody(code, inputField string) string {
	f := "null"
	if inputField != "" {
		f = `"` + inputField + `"`
	}
	// A walk-only itinerary accompanies the error, as observed with OTP.
	return `{"data":{"planConnection":{"routingErrors":[{"code":"` + code + `","inputField":` + f + `,"description":"SECRET upstream text"}],
	 "edges":[{"node":{"start":"2026-10-12T08:00:00+07:00","end":"2026-10-12T11:00:00+07:00","duration":10800,"numberOfTransfers":0,"walkDistance":15000,
	 "legs":[{"mode":"WALK","transitLeg":false,"duration":10800,"distance":15000,"start":{"scheduledTime":"2026-10-12T08:00:00+07:00"},"end":{"scheduledTime":"2026-10-12T11:00:00+07:00"},
	 "from":{"name":"Origin","lat":-6.2,"lon":106.8},"to":{"name":"Destination","lat":-6.1,"lon":106.8}}]}}]}}}`
}

func TestPlanSendsVerifiedQuery(t *testing.T) {
	var got struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/otp/gtfs/v1" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = io.WriteString(w, okResponse)
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv.URL+"/").Plan(context.Background(), query()); err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if !strings.Contains(got.Query, "planConnection(") || strings.Contains(got.Query, " plan(") {
		t.Errorf("query does not use planConnection: %s", got.Query)
	}
	v, _ := json.Marshal(got.Variables)
	want := `{"departAt":"2026-10-12T08:00:00+07:00","destination":{"location":{"coordinate":{"latitude":-6.1205,"longitude":106.8105}}},"first":3,"origin":{"location":{"coordinate":{"latitude":-6.2,"longitude":106.8}}}}`
	if string(v) != want {
		t.Errorf("variables = %s\nwant      %s", v, want)
	}
	if strings.Contains(string(v), "Rumah") {
		t.Error("label must not be sent upstream")
	}
}

func TestPlanMapsResponse(t *testing.T) {
	srv := serve(t, 200, okResponse)
	p, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if p.DataVersion != nil {
		t.Errorf("DataVersion = %+v, want nil", p.DataVersion)
	}
	if len(p.Attributions) != 1 || p.Attributions[0].ID != "test" {
		t.Errorf("attributions = %+v", p.Attributions)
	}
	if len(p.Itineraries) != 1 {
		t.Fatalf("itineraries = %d", len(p.Itineraries))
	}
	it := p.Itineraries[0]
	if !it.Departure.Equal(depart) || !it.Arrival.Equal(depart.Add(40*time.Minute)) ||
		it.Duration != 40*time.Minute || it.Transfers != 0 || it.WalkDistance != 250.4 ||
		it.TimeQuality != tripplan.TimeFrequencyEstimate || len(it.Legs) != 3 {
		t.Fatalf("itinerary = %+v", it)
	}
	walk, bus, last := it.Legs[0], it.Legs[1], it.Legs[2]
	if walk.Type != tripplan.LegWalk || walk.Mode != "walk" || walk.Line != nil ||
		walk.From.Name != "Rumah" || walk.From.StopID != "" || walk.To.StopID != "tj:stop:S1" || walk.Duration != 5*time.Minute {
		t.Errorf("walk leg = %+v", walk)
	}
	if bus.Type != tripplan.LegTransit || bus.Mode != "bus" || bus.Headsign != "Uji B" ||
		*bus.Line != (tripplan.Line{ID: "tj:line:R1", ShortName: "U1", LongName: "Uji A - Uji B"}) ||
		bus.From.StopID != "tj:stop:S1" || bus.To.StopID != "tj:stop:S2" || bus.Distance != 9000 {
		t.Errorf("bus leg = %+v line=%+v", bus, bus.Line)
	}
	if last.To.Name != "" || last.To.StopID != "" {
		t.Errorf("unlabelled destination should have empty name, got %+v", last.To)
	}

	// The mapped plan must satisfy the service's own checks.
	if _, err := tripplan.NewService(newTestClient(t, srv.URL)).Plan(context.Background(), query()); err != nil {
		t.Errorf("service rejected mapped plan: %v", err)
	}
}

func TestPlanNoItinerary(t *testing.T) {
	srv := serve(t, 200, `{"data":{"planConnection":{"routingErrors":[],"edges":[]}}}`)
	_, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
	if !errors.Is(err, tripplan.ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

// withRoutingErrors puts routing errors into a response that also carries
// edges: none, a walk-only itinerary, or a transit itinerary.
func withRoutingErrors(errorsJSON, edges string) string {
	switch edges {
	case "none":
		return `{"data":{"planConnection":{"routingErrors":` + errorsJSON + `,"edges":[]}}}`
	case "walk-only":
		body := routingErrorBody("X", "")
		start := strings.Index(body, `"routingErrors":`)
		end := strings.Index(body, `,
	 "edges"`)
		return body[:start] + `"routingErrors":` + errorsJSON + body[end:]
	default: // transit
		return strings.Replace(okResponse, `"routingErrors":[]`, `"routingErrors":`+errorsJSON, 1)
	}
}

func rerr(code, field string) string {
	f := "null"
	if field != "" {
		f = `"` + field + `"`
	}
	return `{"code":"` + code + `","inputField":` + f + `,"description":"SECRET upstream text"}`
}

// Every value of OTP 2.10.0's RoutingErrorCode enum, plus an unknown one.
func TestPlanRoutingErrors(t *testing.T) {
	cases := []struct {
		code, field string
		check       func(error) bool
	}{
		{"LOCATION_NOT_FOUND", "FROM", isNotRoutable(tripplan.SideOrigin)},
		{"LOCATION_NOT_FOUND", "TO", isNotRoutable(tripplan.SideDestination)},
		{"LOCATION_NOT_FOUND", "", isNotRoutable(tripplan.SideUnknown)},
		{"LOCATION_NOT_FOUND", "VIA", isNotRoutable(tripplan.SideUnknown)},
		{"OUTSIDE_BOUNDS", "FROM", isNotRoutable(tripplan.SideOrigin)},
		{"OUTSIDE_BOUNDS", "TO", isNotRoutable(tripplan.SideDestination)},
		{"NO_STOPS_IN_RANGE", "FROM", isNoRoute},
		{"NO_TRANSIT_CONNECTION", "", isNoRoute},
		{"NO_TRANSIT_CONNECTION_IN_SEARCH_WINDOW", "", isNoRoute},
		{"NO_DIRECT_MODE_CONNECTION", "", isNoRoute},
		{"OUTSIDE_SERVICE_PERIOD", "DATE_TIME", isNoRoute},
		{"WALKING_BETTER_THAN_TRANSIT", "", isNoRoute},
		{"SOME_FUTURE_CODE", "", isNoRoute},
	}
	for _, c := range cases {
		for _, edges := range []string{"none", "walk-only", "transit"} {
			t.Run(c.code+"/"+c.field+"/"+edges, func(t *testing.T) {
				srv := serve(t, 200, withRoutingErrors("["+rerr(c.code, c.field)+"]", edges))
				p, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
				if !c.check(err) {
					t.Fatalf("err = %#v", err)
				}
				if len(p.Itineraries) != 0 {
					t.Errorf("itineraries leaked past routing error: %d", len(p.Itineraries))
				}
				var ooc *tripplan.OutOfCoverageError
				if errors.As(err, &ooc) {
					t.Error("must not classify as OUT_OF_COVERAGE without a service-area check")
				}
				assertNoLeak(t, err)
			})
		}
	}
}

func TestPlanMultipleRoutingErrors(t *testing.T) {
	cases := []struct {
		name   string
		errors string
		check  func(error) bool
	}{
		{"location error wins regardless of order",
			"[" + rerr("WALKING_BETTER_THAN_TRANSIT", "") + "," + rerr("LOCATION_NOT_FOUND", "TO") + "]",
			isNotRoutable(tripplan.SideDestination)},
		{"first location error decides the side",
			"[" + rerr("OUTSIDE_BOUNDS", "FROM") + "," + rerr("LOCATION_NOT_FOUND", "TO") + "]",
			isNotRoutable(tripplan.SideOrigin)},
		{"only non-location errors",
			"[" + rerr("OUTSIDE_SERVICE_PERIOD", "DATE_TIME") + "," + rerr("NO_TRANSIT_CONNECTION", "") + "]",
			isNoRoute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := serve(t, 200, withRoutingErrors(c.errors, "transit"))
			_, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
			if !c.check(err) {
				t.Fatalf("err = %#v", err)
			}
		})
	}
}

func isNotRoutable(s tripplan.Side) func(error) bool {
	return func(err error) bool {
		var lnr *tripplan.LocationNotRoutableError
		return errors.As(err, &lnr) && lnr.Side == s
	}
}

func isNoRoute(err error) bool { return errors.Is(err, tripplan.ErrNoRoute) }

func assertNoLeak(t *testing.T, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), "SECRET") {
		t.Errorf("upstream text leaked into error: %v", err)
	}
}

func TestPlanInvalidUpstreamResponses(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"graphql errors":          {200, `{"errors":[{"message":"SECRET validation error","locations":[{"line":1,"column":2}]}],"data":null}`},
		"graphql errors and data": {200, `{"errors":[{"message":"SECRET"}],"data":{"planConnection":{"routingErrors":[],"edges":[]}}}`},
		"malformed json":          {200, `{"data":{"planConnection":`},
		"not json":                {200, `<html>SECRET</html>`},
		"missing data":            {200, `{}`},
		"null planConnection":     {200, `{"data":{"planConnection":null}}`},
		"edge without node":       {200, `{"data":{"planConnection":{"routingErrors":[],"edges":[{}]}}}`},
		"missing legs":            {200, strings.Replace(okResponse, `"legs":[`, `"legsX":[`, 1)},
		"missing transfers":       {200, strings.Replace(okResponse, `"numberOfTransfers":0,`, ``, 1)},
		"missing scheduledTime":   {200, strings.Replace(okResponse, `{"scheduledTime":"2026-10-12T08:06:00+07:00"}`, `{}`, 1)},
		"bad time format":         {200, strings.Replace(okResponse, `"2026-10-12T08:06:00+07:00"`, `"yesterday"`, 1)},
		"time without offset":     {200, strings.Replace(okResponse, `"2026-10-12T08:06:00+07:00"`, `"2026-10-12T08:06:00"`, 1)},
		"missing place lat":       {200, strings.Replace(okResponse, `"lat":-6.12,`, ``, 1)},
		"transit without route":   {200, strings.Replace(okResponse, `"route":{"gtfsId":"1:R1","shortName":"U1","longName":"Uji A - Uji B","mode":"BUS"}`, `"route":null`, 1)},
		"unexpected feed id":      {200, strings.Replace(okResponse, `"1:R1"`, `"9:R1"`, 1)},
		"unexpected mode":         {200, strings.Replace(okResponse, `"mode":"WALK"`, `"mode":"BICYCLE"`, 1)},
		"wrong field type":        {200, strings.Replace(okResponse, `"duration":2400`, `"duration":"SECRET"`, 1)},
		"http 400":                {400, `{"errors":[{"message":"SECRET"}]}`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, c.status, c.body)
			_, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
			if !errors.Is(err, tripplan.ErrUpstreamInvalid) {
				t.Fatalf("err = %v, want ErrUpstreamInvalid", err)
			}
			assertNoLeak(t, err)
		})
	}
}

func TestPlanUpstreamServerErrorIsUnavailable(t *testing.T) {
	for _, status := range []int{500, 502, 503, 429} {
		srv := serve(t, status, `SECRET`)
		_, err := newTestClient(t, srv.URL).Plan(context.Background(), query())
		if !errors.Is(err, tripplan.ErrRoutingUnavailable) {
			t.Errorf("status %d: err = %v, want ErrRoutingUnavailable", status, err)
		}
		assertNoLeak(t, err)
	}
}

func TestPlanOversizedResponse(t *testing.T) {
	srv := serve(t, 200, okResponse) // ~2.5 KiB
	c := newTestClient(t, srv.URL, func(cfg *Config) { cfg.MaxResponseBytes = 1024 })
	_, err := c.Plan(context.Background(), query())
	if !errors.Is(err, tripplan.ErrUpstreamInvalid) {
		t.Fatalf("err = %v, want ErrUpstreamInvalid", err)
	}

	exact := newTestClient(t, srv.URL, func(cfg *Config) { cfg.MaxResponseBytes = int64(len(okResponse)) })
	if _, err := exact.Plan(context.Background(), query()); err != nil {
		t.Errorf("body exactly at limit: %v", err)
	}
}

func TestPlanConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens there any more

	_, err := newTestClient(t, url).Plan(context.Background(), query())
	if !errors.Is(err, tripplan.ErrRoutingUnavailable) {
		t.Fatalf("err = %v, want ErrRoutingUnavailable", err)
	}
	if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "refused") {
		t.Errorf("transport detail leaked: %v", err)
	}
}

// blockingServer holds requests until the client goes away.
func blockingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reading the body lets the server notice the client hanging up.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPlanUpstreamTimeout(t *testing.T) {
	srv := blockingServer(t)
	c := newTestClient(t, srv.URL, func(cfg *Config) { cfg.Timeout = 50 * time.Millisecond })
	start := time.Now()
	_, err := c.Plan(context.Background(), query())
	if !errors.Is(err, tripplan.ErrRoutingTimeout) {
		t.Fatalf("err = %v, want ErrRoutingTimeout", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("timeout took %v", d)
	}
}

func TestPlanCallerDeadlineShorterThanUpstreamTimeout(t *testing.T) {
	srv := blockingServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := newTestClient(t, srv.URL).Plan(ctx, query())
	if !errors.Is(err, tripplan.ErrRoutingTimeout) {
		t.Fatalf("err = %v, want ErrRoutingTimeout", err)
	}
}

func TestPlanCallerCancellation(t *testing.T) {
	srv := blockingServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	_, err := newTestClient(t, srv.URL).Plan(ctx, query())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if errors.Is(err, tripplan.ErrRoutingTimeout) {
		t.Error("cancellation must not be reported as timeout")
	}
}

func TestNewValidatesConfig(t *testing.T) {
	bad := []Config{
		{BaseURL: "localhost:8082", Attributions: testAttributions()},
		{BaseURL: "ftp://example.test", Attributions: testAttributions()},
		{BaseURL: "http://user:pass@localhost:8082", Attributions: testAttributions()},
		{BaseURL: "http://localhost:8082?x=1", Attributions: testAttributions()},
		{BaseURL: "http://localhost:8082"},
	}
	for _, cfg := range bad {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded, want error", cfg)
		}
	}
	c, err := New(Config{Attributions: testAttributions()})
	if err != nil {
		t.Fatal(err)
	}
	if c.Endpoint() != "http://localhost:8082/otp/gtfs/v1" {
		t.Errorf("default endpoint = %q", c.Endpoint())
	}
}

func TestPilotAttributions(t *testing.T) {
	a := PilotAttributions()
	if len(a) != 2 || a[0].License != "CC-BY-4.0" || a[1].License != "ODbL-1.0" {
		t.Fatalf("attributions = %+v", a)
	}
	for _, x := range a {
		if x.ID == "" || x.Text == "" || x.LicenseURL == "" {
			t.Errorf("incomplete attribution %+v", x)
		}
	}
}
