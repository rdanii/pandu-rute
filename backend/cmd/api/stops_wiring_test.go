package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStops writes a synthetic stops.txt (made-up ids and names).
func writeStops(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stops.txt")
	csv := "stop_id,stop_name,stop_lat,stop_lon\nS1,Halte Uji Wiring,-6.2,106.8\n"
	if err := os.WriteFile(path, []byte(csv), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestServerServesStopSearch(t *testing.T) {
	srv, _, err := newServer(env(map[string]string{"GTFS_STOPS_PATH": writeStops(t)}))
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, srv.Handler, http.MethodGet, "/v1/stops?q=wiring", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body)
	}
	var resp struct {
		Stops        []struct{ ID, Name string }    `json:"stops"`
		Attributions []struct{ ID, License string } `json:"attributions"`
		DataVersion  *struct {
			Sources []struct{ ID, SHA256 string } `json:"sources"`
		} `json:"dataVersion"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Stops) != 1 || resp.Stops[0].ID != "tj:stop:S1" || resp.Stops[0].Name != "Halte Uji Wiring" {
		t.Errorf("stops = %+v", resp.Stops)
	}
	// Stop data comes only from TransJakarta GTFS, so OSM is not credited here.
	if len(resp.Attributions) != 1 || resp.Attributions[0].ID != "transjakarta-gtfs" || resp.Attributions[0].License != "CC-BY-4.0" {
		t.Errorf("attributions = %+v", resp.Attributions)
	}
	if resp.DataVersion == nil || len(resp.DataVersion.Sources) != 1 || len(resp.DataVersion.Sources[0].SHA256) != 64 {
		t.Errorf("dataVersion = %+v", resp.DataVersion)
	}
}

func TestServerStartsWithoutStopData(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"path not set":   nil,
		"file missing":   {"GTFS_STOPS_PATH": filepath.Join(t.TempDir(), "absent.txt")},
		"file malformed": {"GTFS_STOPS_PATH": writeBadStops(t)},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _, err := newServer(env(vars))
			if err != nil {
				t.Fatalf("newServer must not fail without stop data: %v", err)
			}
			rec := do(t, srv.Handler, http.MethodGet, "/v1/stops?q=halte", "")
			if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"STOP_INDEX_UNAVAILABLE"`) {
				t.Errorf("stops = %d %s", rec.Code, rec.Body)
			}
			if h := do(t, srv.Handler, http.MethodGet, "/healthz", ""); h.Code != http.StatusOK {
				t.Errorf("healthz = %d", h.Code)
			}
		})
	}
}

func writeBadStops(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stops.txt")
	if err := os.WriteFile(path, []byte("stop_id,stop_name\nS1,Tanpa Koordinat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
