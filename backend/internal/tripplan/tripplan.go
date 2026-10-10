// Package tripplan holds the app-owned trip-planning model and service.
//
// It knows nothing about HTTP or about any routing engine's wire format.
// Routing engines plug in through the Planner interface; their results are
// translated into these types before they reach the HTTP layer.
package tripplan

import (
	"context"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultMaxItineraries = 3
	MinMaxItineraries     = 1
	MaxMaxItineraries     = 5
	MaxLabelLength        = 100 // runes
)

// Coordinate is a WGS84 position in decimal degrees.
type Coordinate struct {
	Lat float64
	Lon float64
}

// Valid reports whether the coordinate is within WGS84 ranges.
func (c Coordinate) Valid() bool {
	return c.Lat >= -90 && c.Lat <= 90 && c.Lon >= -180 && c.Lon <= 180
}

// Location is a routing endpoint. Label is display text supplied by the
// client; it is echoed back but never geocoded or used for routing.
type Location struct {
	Coordinate Coordinate
	Label      string
}

// Query is a validated trip-planning request.
type Query struct {
	Origin         Location
	Destination    Location
	DepartAt       time.Time
	MaxItineraries int
}

// Problem describes one invalid field.
type Problem struct {
	Field  string
	Reason string
}

// Reasons used in Problem.Reason.
const (
	ReasonRequired      = "required"
	ReasonOutOfRange    = "out_of_range"
	ReasonInvalidType   = "invalid_type"
	ReasonInvalidFormat = "invalid_format"
	ReasonTooLong       = "too_long"
	ReasonTooShort      = "too_short"
	ReasonUnknownField  = "unknown_field"
	ReasonMalformedJSON = "malformed_json"
	ReasonTrailingData  = "trailing_data"
)

// Validate checks domain invariants of the query.
func (q Query) Validate() error {
	var probs []Problem
	check := func(prefix string, l Location) {
		if l.Coordinate.Lat < -90 || l.Coordinate.Lat > 90 {
			probs = append(probs, Problem{prefix + ".lat", ReasonOutOfRange})
		}
		if l.Coordinate.Lon < -180 || l.Coordinate.Lon > 180 {
			probs = append(probs, Problem{prefix + ".lon", ReasonOutOfRange})
		}
		if utf8.RuneCountInString(l.Label) > MaxLabelLength {
			probs = append(probs, Problem{prefix + ".label", ReasonTooLong})
		} else if hasControl(l.Label) {
			probs = append(probs, Problem{prefix + ".label", ReasonInvalidFormat})
		}
	}
	check("origin", q.Origin)
	check("destination", q.Destination)
	if q.DepartAt.IsZero() {
		probs = append(probs, Problem{"departAt", ReasonRequired})
	}
	if q.MaxItineraries < MinMaxItineraries || q.MaxItineraries > MaxMaxItineraries {
		probs = append(probs, Problem{"maxItineraries", ReasonOutOfRange})
	}
	if len(probs) > 0 {
		return &ValidationError{Problems: probs}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

// TimeQuality states how precise itinerary times are.
type TimeQuality string

const (
	// TimeScheduled means times come from exact timetable entries.
	TimeScheduled TimeQuality = "scheduled"
	// TimeFrequencyEstimate means times are derived from headway-based
	// service and must be shown as approximate.
	TimeFrequencyEstimate TimeQuality = "frequency_estimate"
)

// LegType distinguishes transit from walking legs.
type LegType string

const (
	LegTransit LegType = "transit"
	LegWalk    LegType = "walk"
)

// Place is a leg endpoint. StopID is empty for non-stop places.
type Place struct {
	StopID     string
	Name       string
	Coordinate Coordinate
}

// Line identifies a transit line in app-owned terms.
type Line struct {
	ID        string
	ShortName string
	LongName  string
}

// Leg is one segment of an itinerary.
type Leg struct {
	Type     LegType
	Mode     string // e.g. "bus", "walk"
	Line     *Line  // transit legs only
	Headsign string
	From     Place
	To       Place
	Duration time.Duration
	Distance float64 // meters
}

// Itinerary is one way to make the trip.
type Itinerary struct {
	Departure    time.Time
	Arrival      time.Time
	TimeQuality  TimeQuality
	Duration     time.Duration
	Transfers    int
	WalkDistance float64 // meters
	Legs         []Leg
}

// Attribution is a data-source credit that must accompany results.
type Attribution struct {
	ID         string
	Text       string
	License    string
	LicenseURL string
	SourceURL  string
}

// SourceVersion identifies the version of one routing data source.
type SourceVersion struct {
	ID           string
	SHA256       string
	LastModified *time.Time
	RetrievedAt  *time.Time
}

// DataVersion describes the routing data behind a result.
type DataVersion struct {
	Sources      []SourceVersion
	GraphBuiltAt *time.Time
}

// Plan is a planner's answer. DataVersion is nil when the planner cannot
// state it; it is never filled with placeholder values.
type Plan struct {
	Itineraries  []Itinerary
	Attributions []Attribution
	DataVersion  *DataVersion
}

// Planner computes itineraries. Implementations translate a routing
// engine's results into the app-owned types and report failures with the
// errors defined in this package.
type Planner interface {
	Plan(ctx context.Context, q Query) (Plan, error)
}
