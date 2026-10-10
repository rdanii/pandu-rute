package tripplan_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"pandu-rute/backend/internal/tripplan"
	"pandu-rute/backend/internal/tripplan/tripplantest"
)

var depart = time.Date(2026, 10, 12, 8, 0, 0, 0, time.FixedZone("WIB", 7*3600))

func validQuery() tripplan.Query {
	return tripplan.Query{
		Origin:         tripplan.Location{Coordinate: tripplan.Coordinate{Lat: -6.2, Lon: 106.8}},
		Destination:    tripplan.Location{Coordinate: tripplan.Coordinate{Lat: -6.1, Lon: 106.81}},
		DepartAt:       depart,
		MaxItineraries: 3,
	}
}

func TestPlanWithoutPlannerIsUnavailable(t *testing.T) {
	_, err := tripplan.NewService(nil).Plan(context.Background(), validQuery())
	if !errors.Is(err, tripplan.ErrRoutingUnavailable) {
		t.Fatalf("err = %v, want ErrRoutingUnavailable", err)
	}
}

func TestPlanValidatesQueryBeforeCallingPlanner(t *testing.T) {
	f := &tripplantest.FakePlanner{Result: tripplantest.SamplePlan(depart)}
	q := validQuery()
	q.Origin.Coordinate.Lat = 91
	q.MaxItineraries = 6

	_, err := tripplan.NewService(f).Plan(context.Background(), q)
	var ve *tripplan.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	if len(ve.Problems) != 2 {
		t.Errorf("problems = %+v, want 2", ve.Problems)
	}
	if n := len(f.Queries()); n != 0 {
		t.Errorf("planner called %d times, want 0", n)
	}
}

func TestPlanReturnsPlannerResult(t *testing.T) {
	f := &tripplantest.FakePlanner{Result: tripplantest.SamplePlan(depart)}
	got, err := tripplan.NewService(f).Plan(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got.Itineraries) != 1 || got.DataVersion != nil {
		t.Errorf("got %+v", got)
	}
	if qs := f.Queries(); len(qs) != 1 || qs[0].DepartAt != depart {
		t.Errorf("queries = %+v", qs)
	}
}

func TestPlanTruncatesToMaxItineraries(t *testing.T) {
	p := tripplantest.SamplePlan(depart)
	p.Itineraries = []tripplan.Itinerary{
		itin(depart, 20, transit("L1", "A", "B")),
		itin(depart, 25, transit("L2", "A", "B")),
		itin(depart, 30, transit("L3", "A", "B")),
	}
	q := validQuery()
	q.MaxItineraries = 2

	got, err := tripplan.NewService(&tripplantest.FakePlanner{Result: p}).Plan(context.Background(), q)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got.Itineraries) != 2 {
		t.Errorf("itineraries = %d, want 2", len(got.Itineraries))
	}
}

func TestPlanEmptyResultIsNoRoute(t *testing.T) {
	f := &tripplantest.FakePlanner{Result: tripplan.Plan{Attributions: tripplantest.SamplePlan(depart).Attributions}}
	_, err := tripplan.NewService(f).Plan(context.Background(), validQuery())
	if !errors.Is(err, tripplan.ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute", err)
	}
}

func TestPlanRejectsInvalidPlannerOutput(t *testing.T) {
	cases := map[string]func(p *tripplan.Plan){
		"no attributions":          func(p *tripplan.Plan) { p.Attributions = nil },
		"empty attribution text":   func(p *tripplan.Plan) { p.Attributions[0].Text = "" },
		"no legs":                  func(p *tripplan.Plan) { p.Itineraries[0].Legs = nil },
		"arrival before departure": func(p *tripplan.Plan) { p.Itineraries[0].Arrival = depart.Add(-time.Minute) },
		"missing time quality":     func(p *tripplan.Plan) { p.Itineraries[0].TimeQuality = "" },
		"negative transfers":       func(p *tripplan.Plan) { p.Itineraries[0].Transfers = -1 },
		"transit leg without line": func(p *tripplan.Plan) { p.Itineraries[0].Legs[0].Line = nil },
		"unknown leg type":         func(p *tripplan.Plan) { p.Itineraries[0].Legs[1].Type = "ferry" },
		"leg coordinate invalid":   func(p *tripplan.Plan) { p.Itineraries[0].Legs[0].To.Coordinate.Lat = 100 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := tripplantest.SamplePlan(depart)
			mutate(&p)
			_, err := tripplan.NewService(&tripplantest.FakePlanner{Result: p}).Plan(context.Background(), validQuery())
			if !errors.Is(err, tripplan.ErrUpstreamInvalid) {
				t.Fatalf("err = %v, want ErrUpstreamInvalid", err)
			}
		})
	}
}

func TestPlanDeadlineBecomesTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := tripplan.NewService(&tripplantest.FakePlanner{Block: true}).Plan(ctx, validQuery())
	if !errors.Is(err, tripplan.ErrRoutingTimeout) {
		t.Fatalf("err = %v, want ErrRoutingTimeout", err)
	}
}

func TestPlanPassesThroughLocationErrors(t *testing.T) {
	f := &tripplantest.FakePlanner{Err: &tripplan.OutOfCoverageError{Side: tripplan.SideDestination}}
	_, err := tripplan.NewService(f).Plan(context.Background(), validQuery())
	var ooc *tripplan.OutOfCoverageError
	if !errors.As(err, &ooc) || ooc.Side != tripplan.SideDestination {
		t.Fatalf("err = %v, want OutOfCoverageError{destination}", err)
	}
}
