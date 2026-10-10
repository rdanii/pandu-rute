package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"pandu-rute/backend/internal/tripplan"
)

// Synthetic stops with display metadata (made-up ids, names and codes).
const stopsMetaCSV = `stop_id,stop_name,stop_lat,stop_lon,location_type,parent_station,platform_code
ST,Stasiun Contoh,-6.21,106.81,1,,
PL,Stasiun Contoh,-6.211,106.811,0,ST,2
SB,Halte Contoh Biasa,-6.22,106.82,0,,
DG,Halte Contoh Yatim,-6.23,106.83,0,HILANG,
`

func TestStopSearchIncludesMetadataWhenKnown(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsMetaCSV), []tripplan.Attribution{testStopAttribution})
	rec := env.get("q=contoh")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	var body struct {
		Stops []map[string]any `json:"stops"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	for _, s := range body.Stops {
		got[s["id"].(string)] = s
	}
	want := map[string]map[string]any{
		"tj:stop:ST": {"kind": "station"},
		"tj:stop:PL": {"kind": "platform", "platformCode": "2", "parentStationId": "tj:stop:ST"},
		"tj:stop:SB": {"kind": "stop"},
		"tj:stop:DG": {"kind": "stop"}, // dangling parent: still returned, no parentStationId
	}
	if len(got) != len(want) {
		t.Fatalf("got %d stops: %v", len(got), body.Stops)
	}
	for id, w := range want {
		s := got[id]
		for _, k := range []string{"kind", "platformCode", "parentStationId"} {
			v, present := s[k]
			wv, wantPresent := w[k]
			if present != wantPresent || (present && v != wv) {
				t.Errorf("%s: %s = %v (present %v), want %v (present %v)", id, k, v, present, wv, wantPresent)
			}
		}
		for _, k := range []string{"id", "name", "lat", "lon"} {
			if _, ok := s[k]; !ok {
				t.Errorf("%s: required field %s missing", id, k)
			}
		}
	}
	// Two stops share the name; both are kept, the response does not group them.
	if got["tj:stop:ST"]["name"] != got["tj:stop:PL"]["name"] {
		t.Error("fixture expects identical names")
	}
}

func TestStopSearchOmitsUnknownMetadata(t *testing.T) {
	env := newStopsEnv(mustIndex(t, stopsCSV), []tripplan.Attribution{testStopAttribution})
	rec := env.get("q=halte+uji")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	for _, k := range []string{`"kind"`, `"platformCode"`, `"parentStationId"`} {
		if strings.Contains(rec.Body.String(), k) {
			t.Errorf("%s must be omitted for a four-column file: %s", k, rec.Body)
		}
	}
	// Existing fields are unchanged.
	resp := decodeStops(t, rec)
	if len(resp.Stops) != 2 || resp.Stops[0].ID != "tj:stop:B2" || resp.Stops[0].Name != "Halte Uji Beta" ||
		resp.Stops[0].Lat != -6.202 || resp.Stops[0].Lon != 106.802 {
		t.Errorf("stops = %+v", resp.Stops)
	}
}
