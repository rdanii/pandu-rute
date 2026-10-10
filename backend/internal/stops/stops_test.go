package stops

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// All fixtures are synthetic; names and ids are made up.

var bom = string([]byte{0xEF, 0xBB, 0xBF})

func parse(t *testing.T, csv string) *Index {
	t.Helper()
	idx, err := Parse(strings.NewReader(csv), "tj")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return idx
}

func names(ss []Stop) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.Name
	}
	return out
}

func TestParseQuotingUTF8AndColumnOrder(t *testing.T) {
	csv := bom + `stop_lat,stop_code,stop_id,stop_name,stop_lon
-6.2,X1,S1,"Halte ""Uji"", Satu",106.8
-6.21,,S2,Halte Kafé Ñusantara,106.81
  -6.22 ,,  S3  ,  Halte Spasi  , 106.82
`
	idx := parse(t, csv)
	if idx.Len() != 3 {
		t.Fatalf("Len = %d, want 3", idx.Len())
	}
	got := idx.Search("halte", 10)
	want := []Stop{
		{ID: "tj:stop:S3", Name: "Halte Spasi", Lat: -6.22, Lon: 106.82},
		{ID: "tj:stop:S1", Name: `Halte "Uji", Satu`, Lat: -6.2, Lon: 106.8},
		{ID: "tj:stop:S2", Name: "Halte Kafé Ñusantara", Lat: -6.21, Lon: 106.81},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if r := idx.Search("kafé ñus", 10); len(r) != 1 || r[0].ID != "tj:stop:S2" {
		t.Errorf("UTF-8 search = %+v", r)
	}
}

func TestParseSkipsBadRows(t *testing.T) {
	csv := `stop_id,stop_name,stop_lat,stop_lon
OK1,Halte Baik,-6.2,106.8
SHORT,Halte Pendek,-6.2
,Tanpa Id,-6.2,106.8
NONAME,,-6.2,106.8
BADLAT,Halte Lat Huruf,abc,106.8
EMPTYLAT,Halte Lat Kosong,,106.8
BIGLAT,Halte Lat Besar,91,106.8
BIGLON,Halte Lon Besar,-6.2,181
NANLAT,Halte NaN,NaN,106.8
INFLON,Halte Inf,-6.2,Inf
DUP,Halte Ganda Satu,-6.2,106.8
DUP,Halte Ganda Dua,-6.3,106.9
OK2,Halte Baik Dua,-90,180
`
	idx := parse(t, csv)
	st := idx.Stats()
	if st.Rows != 13 || st.Loaded != 2 {
		t.Fatalf("stats = %+v", st)
	}
	want := map[string]int{
		SkipFieldCount: 1, SkipMissingID: 1, SkipMissingName: 1,
		SkipInvalidCoordinates: 6, SkipDuplicateID: 2,
	}
	if !reflect.DeepEqual(st.Skipped, want) {
		t.Errorf("skipped = %v, want %v", st.Skipped, want)
	}
	if got := names(idx.Search("halte", 10)); !reflect.DeepEqual(got, []string{"Halte Baik", "Halte Baik Dua"}) {
		t.Errorf("loaded = %v", got)
	}
	if r := idx.Search("ganda", 10); len(r) != 0 {
		t.Errorf("duplicate id must be excluded entirely, got %+v", r)
	}
}

func TestParseRejectsBrokenFiles(t *testing.T) {
	cases := map[string]string{
		"missing stop_lon column":  "stop_id,stop_name,stop_lat\nS1,Halte,-6.2\n",
		"missing stop_name column": "stop_id,stop_lat,stop_lon\nS1,-6.2,106.8\n",
		"bare quote":               "stop_id,stop_name,stop_lat,stop_lon\nS1,Hal\"te,-6.2,106.8\n",
		"unterminated quote":       "stop_id,stop_name,stop_lat,stop_lon\nS1,\"Halte,-6.2,106.8\n",
		"empty file":               "",
	}
	for name, csv := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(strings.NewReader(csv), "tj"); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := Parse(strings.NewReader("stop_id,stop_name,stop_lat,stop_lon\n,,,\n"), "tj"); !errors.Is(err, ErrNoStops) {
		t.Errorf("no valid rows: err = %v, want ErrNoStops", err)
	}
	if _, err := Parse(strings.NewReader("stop_id,stop_name,stop_lat,stop_lon\nS1,H,1,1\n"), ""); err == nil {
		t.Error("empty namespace accepted")
	}
	_, err := Parse(strings.NewReader("stop_id,stop_name,stop_lat\nS1,Rahasia,-6.2\n"), "tj")
	if err == nil || !strings.Contains(err.Error(), `"stop_lon"`) || strings.Contains(err.Error(), "Rahasia") {
		t.Errorf("missing-column error should name the column and not include row content: %v", err)
	}
}

func TestLoadFileComputesSHA256(t *testing.T) {
	content := []byte("stop_id,stop_name,stop_lat,stop_lon\nS1,Halte Berkas,-6.2,106.8\n")
	path := filepath.Join(t.TempDir(), "stops.txt")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := LoadFile(path, "tj")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if idx.Version() != hex.EncodeToString(sum[:]) {
		t.Errorf("Version = %q", idx.Version())
	}
	if p := parse(t, string(content)); p.Version() != "" {
		t.Errorf("Parse must not invent a version, got %q", p.Version())
	}
}

func TestLoadFileFailures(t *testing.T) {
	if _, err := LoadFile(filepath.Join(t.TempDir(), "absent.txt"), "tj"); err == nil {
		t.Error("missing file accepted")
	}

	path := filepath.Join(t.TempDir(), "stops.txt")
	if err := os.WriteFile(path, []byte("stop_id,stop_name,stop_lat,stop_lon\nS1,Halte,-6.2,106.8\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := maxFileBytes
	maxFileBytes = 10
	defer func() { maxFileBytes = old }()
	if _, err := LoadFile(path, "tj"); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("oversized file: err = %v", err)
	}
}

const rankingCSV = `stop_id,stop_name,stop_lat,stop_lon
Z9,Kampungsenayan,-6.1,106.1
Y8,Bundaran Senayan,-6.1,106.1
X7,Senayan Bank Jakarta,-6.1,106.1
W6,Senayan,-6.1,106.1
V5,Senayan Timur,-6.1,106.1
U4,Senayan Barat,-6.1,106.1
T3,Pasar Baru,-6.1,106.1
B2,Senayan Barat,-6.1,106.1
`

func TestSearchRanking(t *testing.T) {
	idx := parse(t, rankingCSV)
	got := idx.Search("senayan", 20)
	want := []string{
		"tj:stop:W6", // exact
		"tj:stop:B2", // prefix "senayan barat"; same name as U4, tie broken by ID
		"tj:stop:U4",
		"tj:stop:V5", // prefix "senayan timur": same length, later in name order
		"tj:stop:X7", // prefix, longer name
		"tj:stop:Y8", // word-prefix
		"tj:stop:Z9", // substring only
	}
	ids := make([]string, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v\nwant    %v", ids, want)
	}
}

func TestSearchIsDeterministicAcrossInputOrder(t *testing.T) {
	lines := strings.Split(strings.TrimSpace(rankingCSV), "\n")
	reversed := []string{lines[0]}
	for i := len(lines) - 1; i >= 1; i-- {
		reversed = append(reversed, lines[i])
	}
	a := parse(t, rankingCSV).Search("senayan", 20)
	b := parse(t, strings.Join(reversed, "\n")+"\n").Search("senayan", 20)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("order depends on input order:\n%v\n%v", a, b)
	}
	for i := 0; i < 10; i++ {
		if c := parse(t, rankingCSV).Search("senayan", 20); !reflect.DeepEqual(a, c) {
			t.Fatal("repeated search differs")
		}
	}
}

func TestSearchNormalisationAndOriginalNames(t *testing.T) {
	idx := parse(t, rankingCSV)
	r := idx.Search("  SENAYAN-bank ", 5)
	if len(r) != 1 || r[0].Name != "Senayan Bank Jakarta" {
		t.Fatalf("got %+v", r)
	}
	if r := idx.Search("st. senayan", 5); len(r) != 0 {
		t.Errorf("abbreviations must not be expanded, got %+v", r)
	}
}

func TestSearchEmptyAndLimits(t *testing.T) {
	idx := parse(t, rankingCSV)
	for _, q := range []string{"tidakada", "--", "   "} {
		if r := idx.Search(q, 10); r == nil || len(r) != 0 {
			t.Errorf("Search(%q) = %#v, want empty non-nil slice", q, r)
		}
	}
	if r := idx.Search("senayan", 2); len(r) != 2 || r[0].ID != "tj:stop:W6" {
		t.Errorf("limit 2 = %+v", r)
	}
	if r := idx.Search("senayan", 0); len(r) != 0 {
		t.Errorf("limit 0 = %+v", r)
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"  Halte  BUNDARAN   HI ": "halte bundaran hi",
		"Kota/Bambu-Utara (2)":    "kota bambu utara 2",
		"Kafé":                    "kafé",
		"--":                      "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
