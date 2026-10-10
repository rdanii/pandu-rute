package stops

import (
	"reflect"
	"strings"
	"testing"
)

// Synthetic fixture (made-up ids, names and codes). P0 is listed before its
// parent ST2 to show parent resolution does not depend on row order.
const metadataCSV = `stop_id,stop_name,stop_lat,stop_lon,location_type,parent_station,platform_code
P0,Peron Awal,-6.10,106.80,0,ST2,1
ST1,Stasiun Uji Satu,-6.11,106.81,1,,
P1,Peron Satu A,-6.12,106.82,0,ST1,2
P2,Peron Satu B,-6.13,106.83,,ST1,Ü-3
S1,Halte Biasa,-6.14,106.84,0,,
D1,Halte Induk Hilang,-6.15,106.85,0,TIDAKADA,
N1,Halte Induk Bukan Stasiun,-6.16,106.86,0,S1,
U3,Halte Tipe Tiga,-6.17,106.87,3,ST1,
UX,Halte Tipe Huruf,-6.18,106.88,x,,
U01,Halte Tipe Nol Satu,-6.19,106.89,01,,
B1,Halte Kode Kosong,-6.20,106.90,0,,"   "
SP,Stasiun Berinduk,-6.21,106.91,1,ST1,
ST2,Stasiun Uji Dua,-6.22,106.92, 1 ,,
SK,Stasiun Koordinat Rusak,abc,106.93,1,,
PK,Peron Induk Dilewati,-6.23,106.94,0,SK,
DUP,Stasiun Ganda,-6.24,106.95,1,,
DUP,Stasiun Ganda,-6.25,106.96,1,,
PD,Peron Induk Ganda,-6.26,106.97,0,DUP,
`

func byID(t *testing.T, idx *Index) map[string]Stop {
	t.Helper()
	out := map[string]Stop{}
	for _, e := range idx.entries {
		out[e.stop.ID] = e.stop
	}
	return out
}

func TestMetadataDerivation(t *testing.T) {
	idx := parse(t, metadataCSV)
	got := byID(t, idx)
	type meta struct {
		kind     Kind
		platform string
		parent   string
	}
	want := map[string]meta{
		"tj:stop:ST1": {KindStation, "", ""},
		"tj:stop:ST2": {KindStation, "", ""}, // " 1 " is trimmed
		"tj:stop:SP":  {KindStation, "", ""}, // a station's own parent is ignored
		"tj:stop:P0":  {KindPlatform, "1", "tj:stop:ST2"},
		"tj:stop:P1":  {KindPlatform, "2", "tj:stop:ST1"},
		"tj:stop:P2":  {KindPlatform, "Ü-3", "tj:stop:ST1"}, // empty location_type = 0
		"tj:stop:S1":  {KindStop, "", ""},
		"tj:stop:D1":  {KindStop, "", ""}, // dangling parent
		"tj:stop:N1":  {KindStop, "", ""}, // parent is not a station
		"tj:stop:PK":  {KindStop, "", ""}, // parent row was skipped (bad coordinates)
		"tj:stop:PD":  {KindStop, "", ""}, // parent id was duplicated, so not loaded
		"tj:stop:U3":  {"", "", ""},       // unrecognised location_type
		"tj:stop:UX":  {"", "", ""},       // malformed location_type
		"tj:stop:U01": {"", "", ""},       // not exactly 0 or 1
		"tj:stop:B1":  {KindStop, "", ""}, // whitespace-only platform_code
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d stops, want %d: %v", len(got), len(want), got)
	}
	for id, w := range want {
		s, ok := got[id]
		if !ok {
			t.Errorf("%s missing: rows with bad metadata must stay searchable", id)
			continue
		}
		if (meta{s.Kind, s.PlatformCode, s.ParentStationID}) != w {
			t.Errorf("%s: got {%q %q %q}, want %+v", id, s.Kind, s.PlatformCode, s.ParentStationID, w)
		}
	}

	st := idx.Stats()
	if st.UnknownLocationType != 3 {
		t.Errorf("UnknownLocationType = %d, want 3", st.UnknownLocationType)
	}
	// D1 (TIDAKADA), PK (skipped SK), PD (duplicated DUP); SP's parent ST1 is valid.
	if st.DanglingParent != 3 {
		t.Errorf("DanglingParent = %d, want 3", st.DanglingParent)
	}
	// N1 -> S1 is a stop. U3 -> ST1 is a station, so it is not counted.
	if st.NonStationParent != 1 {
		t.Errorf("NonStationParent = %d, want 1", st.NonStationParent)
	}
	if st.Skipped[SkipInvalidCoordinates] != 1 || st.Skipped[SkipDuplicateID] != 2 {
		t.Errorf("skipped = %v", st.Skipped)
	}
}

func TestPlatformCodePreservedAsGiven(t *testing.T) {
	csv := "stop_id,stop_name,stop_lat,stop_lon,platform_code\n" +
		"A,Halte A,-6.1,106.8, B \n" +
		"Q,Halte Q,-6.1,106.8,\"Jalur \"\"2\"\", Utara\"\n"
	got := byID(t, parse(t, csv))
	if got["tj:stop:A"].PlatformCode != " B " {
		t.Errorf("non-blank platform_code must be preserved as given, got %q", got["tj:stop:A"].PlatformCode)
	}
	if got["tj:stop:Q"].PlatformCode != `Jalur "2", Utara` {
		t.Errorf("quoted platform_code = %q", got["tj:stop:Q"].PlatformCode)
	}
	if got["tj:stop:A"].Kind != "" {
		t.Errorf("no location_type column: kind must be omitted, got %q", got["tj:stop:A"].Kind)
	}
}

func TestMissingOptionalColumns(t *testing.T) {
	cases := map[string]string{
		"four columns only": "stop_id,stop_name,stop_lat,stop_lon\nS1,Halte,-6.1,106.8\nS2,Halte Dua,-6.2,106.9\n",
		"parent without location_type": "stop_id,stop_name,stop_lat,stop_lon,parent_station\n" +
			"S1,Halte,-6.1,106.8,S2\nS2,Halte Dua,-6.2,106.9,\n",
		"location_type only": "stop_id,stop_name,stop_lat,stop_lon,location_type\nS1,Halte,-6.1,106.8,1\nS2,Halte Dua,-6.2,106.9,0\n",
	}
	for name, csv := range cases {
		t.Run(name, func(t *testing.T) {
			idx := parse(t, csv)
			if idx.Len() != 2 {
				t.Fatalf("Len = %d, want 2", idx.Len())
			}
			for _, s := range byID(t, idx) {
				if s.PlatformCode != "" || s.ParentStationID != "" {
					t.Errorf("unexpected metadata %+v", s)
				}
				hasLT := strings.Contains(strings.SplitN(csv, "\n", 2)[0], "location_type")
				if !hasLT && s.Kind != "" {
					t.Errorf("kind must be omitted without location_type, got %+v", s)
				}
			}
			if idx.Stats().NonStationParent != 0 || idx.Stats().DanglingParent != 0 {
				t.Errorf("stats = %+v", idx.Stats())
			}
		})
	}
	// location_type alone still classifies stations and stops.
	got := byID(t, parse(t, cases["location_type only"]))
	if got["tj:stop:S1"].Kind != KindStation || got["tj:stop:S2"].Kind != KindStop {
		t.Errorf("location_type only: %+v", got)
	}
}

func TestParentResolutionIndependentOfRowOrder(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(metadataCSV), "\n")
	reversed := []string{lines[0]}
	for i := len(lines) - 1; i >= 1; i-- {
		reversed = append(reversed, lines[i])
	}
	a := byID(t, parse(t, metadataCSV))
	b := byID(t, parse(t, strings.Join(reversed, "\n")+"\n"))
	if !reflect.DeepEqual(a, b) {
		t.Fatal("metadata depends on row order")
	}
}

// Adding the optional columns must not change which rows load, their core
// fields, or the search order.
func TestMetadataDoesNotChangeSearch(t *testing.T) {
	withMeta := "stop_id,stop_name,stop_lat,stop_lon,location_type,parent_station,platform_code\n"
	plain := "stop_id,stop_name,stop_lat,stop_lon\n"
	for _, line := range strings.Split(strings.TrimSpace(rankingCSV), "\n")[1:] {
		plain += line + "\n"
		withMeta += line + ",1,,P\n"
	}
	a, b := parse(t, plain), parse(t, withMeta)
	if a.Len() != b.Len() {
		t.Fatalf("Len %d vs %d", a.Len(), b.Len())
	}
	for _, q := range []string{"senayan", "barat", "pasar baru", "an"} {
		ra, rb := a.Search(q, 20), b.Search(q, 20)
		if len(ra) != len(rb) {
			t.Fatalf("%q: %d vs %d results", q, len(ra), len(rb))
		}
		for i := range ra {
			x, y := ra[i], rb[i]
			if x.ID != y.ID || x.Name != y.Name || x.Lat != y.Lat || x.Lon != y.Lon {
				t.Errorf("%q #%d: %+v vs %+v", q, i, x, y)
			}
			if y.Kind != KindStation || y.PlatformCode != "P" {
				t.Errorf("%q #%d: metadata missing: %+v", q, i, y)
			}
		}
	}
}
