// Package stops loads GTFS stops into an in-memory index and searches it by
// name. It knows nothing about HTTP or routing engines.
//
// Only the standard GTFS columns stop_id, stop_name, stop_lat and stop_lon
// are used; other columns are ignored.
package stops

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Stop is a searchable stop with an app-owned ID. Kind, PlatformCode and
// ParentStationID are optional display metadata; empty means unknown.
type Stop struct {
	ID   string // "<namespace>:stop:<stop_id>"
	Name string // stop_name exactly as in the feed
	Lat  float64
	Lon  float64

	Kind            Kind   // "" when location_type is absent or unrecognised
	PlatformCode    string // platform_code as in the feed, "" when blank
	ParentStationID string // "<namespace>:stop:<parent>" only for a loaded station
}

// Kind classifies a stop for display, following GTFS location_type.
type Kind string

const (
	KindStation  Kind = "station"  // location_type 1
	KindPlatform Kind = "platform" // location_type 0/empty with a loaded station as parent
	KindStop     Kind = "stop"     // location_type 0/empty without one
)

// Reasons a row was skipped while loading.
const (
	SkipFieldCount         = "field_count"
	SkipMissingID          = "missing_id"
	SkipMissingName        = "missing_name"
	SkipInvalidCoordinates = "invalid_coordinates"
	SkipDuplicateID        = "duplicate_id"
)

// Stats summarises a load. It never contains row content.
type Stats struct {
	Rows    int
	Loaded  int
	Skipped map[string]int

	// Metadata counts over loaded stops. None of these remove a stop.
	UnknownLocationType int // location_type present but not 0, 1 or empty
	DanglingParent      int // parent_station not among the loaded stops
	NonStationParent    int // parent_station is a loaded stop that is not a station
}

var (
	// ErrNoStops means the file had no usable rows.
	ErrNoStops = errors.New("stops: no valid stops")

	requiredColumns = []string{"stop_id", "stop_name", "stop_lat", "stop_lon"}

	// maxFileBytes guards against loading an unexpectedly large file.
	maxFileBytes int64 = 64 << 20
)

type entry struct {
	stop  Stop
	norm  string // normalised name
	runes int    // rune count of norm, for tie-breaking
}

// Index is an immutable, searchable set of stops.
type Index struct {
	entries []entry
	version string
	stats   Stats
}

// LoadFile reads a GTFS stops.txt file. The returned index's Version is the
// SHA-256 of the exact bytes loaded.
func LoadFile(path, namespace string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("stops: open: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("stops: read: %w", err)
	}
	if int64(len(data)) > maxFileBytes {
		return nil, fmt.Errorf("stops: file exceeds %d bytes", maxFileBytes)
	}
	idx, err := Parse(bytes.NewReader(data), namespace)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	idx.version = hex.EncodeToString(sum[:])
	return idx, nil
}

// Parse reads GTFS stops CSV. Rows with a wrong field count, an empty
// stop_id or stop_name, or missing/invalid coordinates are skipped. Every
// row of a stop_id that occurs more than once is skipped, because it is
// unclear which one is correct. A CSV syntax error fails the whole load.
// The returned index has an empty Version.
//
// The optional columns location_type, parent_station and platform_code only
// add display metadata (see Stop); they never cause a row to be skipped.
// Parent references are resolved after all rows are loaded, so row order
// does not matter.
func Parse(r io.Reader, namespace string) (*Index, error) {
	if namespace == "" {
		return nil, errors.New("stops: namespace is required")
	}
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1 // checked per row below

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("stops: read header: %w", err)
	}
	col := make(map[string]int, len(header))
	for i, h := range header {
		if i == 0 {
			h = strings.TrimPrefix(h, "\xef\xbb\xbf") // UTF-8 byte order mark
		}
		col[strings.TrimSpace(h)] = i
	}
	for _, c := range requiredColumns {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("stops: missing required column %q", c)
		}
	}

	optional := func(name string) int {
		if i, ok := col[name]; ok {
			return i
		}
		return -1
	}
	ltCol, parentCol, platformCol := optional("location_type"), optional("parent_station"), optional("platform_code")

	type row struct {
		stop     Stop
		locType  string // trimmed location_type
		parentID string // trimmed parent_station (feed id)
	}
	stats := Stats{Skipped: map[string]int{}}
	var rows []row
	seen := map[string]int{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				return nil, fmt.Errorf("stops: malformed CSV at line %d", pe.Line)
			}
			return nil, fmt.Errorf("stops: read: %w", err)
		}
		stats.Rows++
		if len(rec) != len(header) {
			stats.Skipped[SkipFieldCount]++
			continue
		}
		id := strings.TrimSpace(rec[col["stop_id"]])
		name := strings.TrimSpace(rec[col["stop_name"]])
		lat, latOK := coordinate(rec[col["stop_lat"]], 90)
		lon, lonOK := coordinate(rec[col["stop_lon"]], 180)
		switch {
		case id == "":
			stats.Skipped[SkipMissingID]++
			continue
		case name == "":
			stats.Skipped[SkipMissingName]++
			continue
		case !latOK || !lonOK:
			stats.Skipped[SkipInvalidCoordinates]++
			continue
		}
		seen[id]++
		r := row{stop: Stop{ID: id, Name: name, Lat: lat, Lon: lon}}
		if ltCol >= 0 {
			r.locType = strings.TrimSpace(rec[ltCol])
		}
		if parentCol >= 0 {
			r.parentID = strings.TrimSpace(rec[parentCol])
		}
		if platformCol >= 0 && strings.TrimSpace(rec[platformCol]) != "" {
			r.stop.PlatformCode = rec[platformCol]
		}
		rows = append(rows, r)
	}

	// Loaded stops (after duplicate removal) and which of them are stations.
	loaded := make([]row, 0, len(rows))
	isStation := map[string]bool{}
	for _, r := range rows {
		if seen[r.stop.ID] > 1 {
			stats.Skipped[SkipDuplicateID]++
			continue
		}
		loaded = append(loaded, r)
		isStation[r.stop.ID] = ltCol >= 0 && r.locType == "1"
	}

	idx := &Index{}
	for _, r := range loaded {
		s := r.stop
		if r.parentID != "" {
			switch station, ok := isStation[r.parentID]; {
			case !ok:
				stats.DanglingParent++
			case !station && ltCol >= 0: // without location_type, station status is unknown
				stats.NonStationParent++
			}
		}
		if ltCol >= 0 {
			switch r.locType {
			case "1":
				s.Kind = KindStation
			case "0", "":
				s.Kind = KindStop
				if r.parentID != "" && isStation[r.parentID] {
					s.Kind = KindPlatform
					s.ParentStationID = namespace + ":stop:" + r.parentID
				}
			default:
				stats.UnknownLocationType++
			}
		}
		s.ID = namespace + ":stop:" + s.ID
		norm := Normalize(s.Name)
		idx.entries = append(idx.entries, entry{stop: s, norm: norm, runes: utf8.RuneCountInString(norm)})
	}
	stats.Loaded = len(idx.entries)
	idx.stats = stats
	if len(idx.entries) == 0 {
		return nil, ErrNoStops
	}
	return idx, nil
}

func coordinate(s string, bound float64) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < -bound || v > bound {
		return 0, false
	}
	return v, true
}

// Version is the SHA-256 (hex) of the loaded file, or "" if unknown.
func (ix *Index) Version() string { return ix.version }

// Stats describes the load.
func (ix *Index) Stats() Stats { return ix.stats }

// Len is the number of searchable stops.
func (ix *Index) Len() int { return len(ix.entries) }

// Normalize lowercases s and turns every run of characters that are not
// letters or digits into a single space. No abbreviations are expanded and
// no diacritics are removed.
func Normalize(s string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Match ranks, best first.
const (
	rankExact = iota
	rankPrefix
	rankWordPrefix
	rankSubstring
)

// Search returns at most limit stops whose normalised name contains the
// normalised query, ordered by rank (exact, prefix, word-prefix,
// substring), then shorter normalised name, then normalised name, then ID.
func (ix *Index) Search(query string, limit int) []Stop {
	q := Normalize(query)
	if q == "" || limit <= 0 {
		return []Stop{}
	}
	type hit struct {
		rank int
		e    *entry
	}
	var hits []hit
	for i := range ix.entries {
		e := &ix.entries[i]
		switch {
		case e.norm == q:
			hits = append(hits, hit{rankExact, e})
		case strings.HasPrefix(e.norm, q):
			hits = append(hits, hit{rankPrefix, e})
		case strings.Contains(e.norm, " "+q):
			hits = append(hits, hit{rankWordPrefix, e})
		case strings.Contains(e.norm, q):
			hits = append(hits, hit{rankSubstring, e})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.e.runes != b.e.runes {
			return a.e.runes < b.e.runes
		}
		if a.e.norm != b.e.norm {
			return a.e.norm < b.e.norm
		}
		return a.e.stop.ID < b.e.stop.ID
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Stop, len(hits))
	for i, h := range hits {
		out[i] = h.e.stop
	}
	return out
}
