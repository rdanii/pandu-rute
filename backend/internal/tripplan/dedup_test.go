package tripplan_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"pandu-rute/backend/internal/tripplan"
	"pandu-rute/backend/internal/tripplan/tripplantest"
)

// Synthetic builders; ids are made up.

func stop(id string) tripplan.Place {
	return tripplan.Place{StopID: "test:stop:" + id, Name: "Halte " + id, Coordinate: tripplan.Coordinate{Lat: -6.2, Lon: 106.8}}
}

var endpoint = tripplan.Place{Coordinate: tripplan.Coordinate{Lat: -6.2, Lon: 106.8}}

func transit(line, from, to string) tripplan.Leg {
	return tripplan.Leg{Type: tripplan.LegTransit, Mode: "bus",
		Line: &tripplan.Line{ID: "test:line:" + line, ShortName: line},
		From: stop(from), To: stop(to), Duration: 10 * time.Minute, Distance: 3000}
}

func walk(from, to tripplan.Place, meters float64) tripplan.Leg {
	return tripplan.Leg{Type: tripplan.LegWalk, Mode: "walk", From: from, To: to,
		Duration: time.Duration(meters) * time.Second, Distance: meters}
}

// itin builds an itinerary departing at dep and arriving after minutes.
func itin(dep time.Time, minutes int, legs ...tripplan.Leg) tripplan.Itinerary {
	it := tripplan.Itinerary{
		Departure: dep, Arrival: dep.Add(time.Duration(minutes) * time.Minute),
		TimeQuality: tripplan.TimeFrequencyEstimate, Duration: time.Duration(minutes) * time.Minute,
		Legs: legs,
	}
	n := 0
	for _, l := range legs {
		if l.Type == tripplan.LegTransit {
			n++
		} else {
			it.WalkDistance += l.Distance
		}
	}
	if n > 1 {
		it.Transfers = n - 1
	}
	return it
}

func TestDeduplicateMergesMinuteShiftedCopies(t *testing.T) {
	legs := []tripplan.Leg{walk(endpoint, stop("A"), 300), transit("1", "A", "B"), walk(stop("B"), endpoint, 100)}
	in := []tripplan.Itinerary{
		itin(depart, 70, legs...),
		itin(depart.Add(time.Minute), 70, legs...),
		itin(depart.Add(2*time.Minute), 70, legs...),
	}
	out := tripplan.Deduplicate(in)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if !out[0].Departure.Equal(depart) {
		t.Errorf("representative departs %v, want earliest arrival (%v)", out[0].Departure, depart)
	}
}

func TestDeduplicateIgnoresWalkDistanceAndHeadsignForKey(t *testing.T) {
	a := itin(depart, 40, walk(endpoint, stop("A"), 300), transit("1", "A", "B"))
	b := itin(depart.Add(time.Minute), 40, walk(endpoint, stop("A"), 320), transit("1", "A", "B"))
	b.Legs[1].Headsign = "Arah lain"
	b.Legs[1].Line.ShortName = "1X"
	if out := tripplan.Deduplicate([]tripplan.Itinerary{a, b}); len(out) != 1 {
		t.Fatalf("len = %d, want 1 (same structure)", len(out))
	}
}

func TestDeduplicateKeepsDifferentJourneys(t *testing.T) {
	base := itin(depart, 40, transit("1", "A", "B"))
	cases := map[string]tripplan.Itinerary{
		"different line":         itin(depart, 40, transit("2", "A", "B")),
		"different boarding":     itin(depart, 40, transit("1", "C", "B")),
		"different alighting":    itin(depart, 40, transit("1", "A", "D")),
		"transfer sequence":      itin(depart, 40, transit("1", "A", "X"), transit("9", "X", "B")),
		"walk access added":      itin(depart, 40, walk(endpoint, stop("A"), 200), transit("1", "A", "B")),
		"walk egress added":      itin(depart, 40, transit("1", "A", "B"), walk(stop("B"), endpoint, 200)),
		"walk transfer vs none":  itin(depart, 40, transit("1", "A", "X"), walk(stop("X"), stop("Y"), 150), transit("9", "Y", "B")),
		"same lines other order": itin(depart, 40, transit("9", "A", "X"), transit("1", "X", "B")),
	}
	for name, other := range cases {
		t.Run(name, func(t *testing.T) {
			if out := tripplan.Deduplicate([]tripplan.Itinerary{base, other}); len(out) != 2 {
				t.Fatalf("len = %d, want 2", len(out))
			}
		})
	}

	// The two transfer variants must also stay distinct from each other.
	viaWalk := cases["walk transfer vs none"]
	direct := cases["transfer sequence"]
	if out := tripplan.Deduplicate([]tripplan.Itinerary{direct, viaWalk}); len(out) != 2 {
		t.Errorf("transfer with and without walk merged")
	}
}

func TestDeduplicateRepresentativeSelection(t *testing.T) {
	legs := []tripplan.Leg{transit("1", "A", "B")}
	early := itin(depart, 30, legs...)
	late := itin(depart, 35, legs...)

	fewerTransfers := itin(depart, 30, legs...)
	moreTransfers := itin(depart, 30, legs...)
	moreTransfers.Transfers = 1 // same arrival, worse transfer count

	shortWalk := itin(depart, 30, legs...)
	longWalk := itin(depart, 30, legs...)
	longWalk.WalkDistance = 200

	cases := []struct {
		name string
		in   []tripplan.Itinerary
		want tripplan.Itinerary
	}{
		{"earliest arrival wins", []tripplan.Itinerary{late, early}, early},
		{"then fewer transfers", []tripplan.Itinerary{moreTransfers, fewerTransfers}, fewerTransfers},
		{"then shorter walk", []tripplan.Itinerary{longWalk, shortWalk}, shortWalk},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := tripplan.Deduplicate(c.in)
			if len(out) != 1 || !reflect.DeepEqual(out[0], c.want) {
				t.Fatalf("got %+v, want %+v", out, c.want)
			}
		})
	}

	// Full tie: the first one in input order is kept, regardless of how often
	// the function runs.
	first := itin(depart, 30, legs...)
	first.Legs = []tripplan.Leg{transit("1", "A", "B")}
	first.Legs[0].Headsign = "pertama"
	second := itin(depart, 30, transit("1", "A", "B"))
	second.Legs[0].Headsign = "kedua"
	for i := 0; i < 20; i++ {
		out := tripplan.Deduplicate([]tripplan.Itinerary{first, second})
		if len(out) != 1 || out[0].Legs[0].Headsign != "pertama" {
			t.Fatalf("tie not resolved by input order: %+v", out)
		}
	}
}

func TestDeduplicateKeepsGroupOrder(t *testing.T) {
	a1 := itin(depart, 50, transit("A", "1", "2"))
	b := itin(depart, 20, transit("B", "1", "2"))
	a2 := itin(depart, 40, transit("A", "1", "2"))
	out := tripplan.Deduplicate([]tripplan.Itinerary{a1, b, a2})
	if len(out) != 2 || out[0].Legs[0].Line.ID != "test:line:A" || out[1].Legs[0].Line.ID != "test:line:B" {
		t.Fatalf("order = %+v", out)
	}
	if !out[0].Arrival.Equal(a2.Arrival) {
		t.Errorf("group A representative should be the earlier arrival")
	}
}

func TestDeduplicateEmptyAndUnkeyable(t *testing.T) {
	if out := tripplan.Deduplicate(nil); len(out) != 0 {
		t.Errorf("nil -> %+v", out)
	}
	if out := tripplan.Deduplicate([]tripplan.Itinerary{}); len(out) != 0 {
		t.Errorf("empty -> %+v", out)
	}

	noStop := itin(depart, 30, transit("1", "A", "B"))
	noStop.Legs[0].From.StopID = ""
	noLine := itin(depart, 30, transit("1", "A", "B"))
	noLine.Legs[0].Line = nil
	noLegs := itin(depart, 30)
	unknownType := itin(depart, 30, transit("1", "A", "B"))
	unknownType.Legs[0].Type = "ferry"

	for name, bad := range map[string]tripplan.Itinerary{"no stop id": noStop, "no line": noLine, "no legs": noLegs, "unknown leg type": unknownType} {
		t.Run(name, func(t *testing.T) {
			out := tripplan.Deduplicate([]tripplan.Itinerary{bad, bad})
			if len(out) != 2 {
				t.Errorf("unkeyable itineraries must not be merged, got %d", len(out))
			}
		})
	}
}

func TestDeduplicateDoesNotMutateInput(t *testing.T) {
	legs := []tripplan.Leg{transit("1", "A", "B")}
	in := []tripplan.Itinerary{itin(depart, 40, legs...), itin(depart, 30, legs...)}
	before := append([]tripplan.Itinerary(nil), in...)
	tripplan.Deduplicate(in)
	if !reflect.DeepEqual(in, before) {
		t.Error("input slice modified")
	}
}

// --- Service integration -------------------------------------------------

func planWith(t *testing.T, its []tripplan.Itinerary, max int, opts ...tripplan.Option) (tripplan.Plan, error) {
	t.Helper()
	p := tripplantest.SamplePlan(depart)
	p.Itineraries = its
	q := validQuery()
	q.MaxItineraries = max
	return tripplan.NewService(&tripplantest.FakePlanner{Result: p}, opts...).Plan(context.Background(), q)
}

func TestServiceDeduplicatesBeforeTruncating(t *testing.T) {
	dup := []tripplan.Leg{transit("1", "A", "B")}
	got, err := planWith(t, []tripplan.Itinerary{
		itin(depart, 30, dup...),
		itin(depart.Add(time.Minute), 30, dup...),
		itin(depart, 45, transit("2", "A", "B")),
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Itineraries) != 2 || got.Itineraries[1].Legs[0].Line.ID != "test:line:2" {
		t.Fatalf("itineraries = %+v", got.Itineraries)
	}
}

func TestServiceDropsWalkOnlyByDefault(t *testing.T) {
	walkOnly := itin(depart, 180, walk(endpoint, endpoint, 15000))
	withAccess := itin(depart, 40, walk(endpoint, stop("A"), 300), transit("1", "A", "B"), walk(stop("B"), endpoint, 100))

	got, err := planWith(t, []tripplan.Itinerary{walkOnly, withAccess}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Itineraries) != 1 || len(got.Itineraries[0].Legs) != 3 {
		t.Fatalf("want only the transit itinerary with its walk legs intact, got %+v", got.Itineraries)
	}

	_, err = planWith(t, []tripplan.Itinerary{walkOnly}, 3)
	if !errors.Is(err, tripplan.ErrNoRoute) {
		t.Fatalf("walk-only result: err = %v, want ErrNoRoute", err)
	}
	var ooc *tripplan.OutOfCoverageError
	if errors.As(err, &ooc) {
		t.Error("walk-only must not become OUT_OF_COVERAGE")
	}
}

func TestServiceWalkOnlyPolicySeam(t *testing.T) {
	short := itin(depart, 5, walk(endpoint, endpoint, 300))
	long := itin(depart, 180, walk(endpoint, endpoint, 15000))
	policy := func(it tripplan.Itinerary) bool { return it.WalkDistance <= 500 } // test-only value

	got, err := planWith(t, []tripplan.Itinerary{long, short}, 3, tripplan.WithWalkOnlyPolicy(policy))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Itineraries) != 1 || got.Itineraries[0].WalkDistance != 300 {
		t.Fatalf("itineraries = %+v", got.Itineraries)
	}

	// A nil policy keeps the default.
	if _, err := planWith(t, []tripplan.Itinerary{short}, 3, tripplan.WithWalkOnlyPolicy(nil)); !errors.Is(err, tripplan.ErrNoRoute) {
		t.Errorf("nil policy: err = %v, want ErrNoRoute", err)
	}
}

func TestServiceStillValidatesBeforeFiltering(t *testing.T) {
	bad := itin(depart, 180, walk(endpoint, endpoint, 15000))
	bad.TimeQuality = ""
	if _, err := planWith(t, []tripplan.Itinerary{bad}, 3); !errors.Is(err, tripplan.ErrUpstreamInvalid) {
		t.Fatalf("err = %v, want ErrUpstreamInvalid", err)
	}
}
