// Package tripplantest provides a deterministic Planner for tests.
package tripplantest

import (
	"context"
	"sync"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

// SamplePlan returns a valid plan built from obviously synthetic test data
// (not a real stop, line or schedule) with one transit and one walk leg.
func SamplePlan(depart time.Time) tripplan.Plan {
	a := tripplan.Place{StopID: "test:stop:A", Name: "Halte Contoh A", Coordinate: tripplan.Coordinate{Lat: -6.2, Lon: 106.8}}
	b := tripplan.Place{StopID: "test:stop:B", Name: "Halte Contoh B", Coordinate: tripplan.Coordinate{Lat: -6.1, Lon: 106.8}}
	end := tripplan.Place{Name: "Tujuan", Coordinate: tripplan.Coordinate{Lat: -6.1, Lon: 106.81}}
	return tripplan.Plan{
		Itineraries: []tripplan.Itinerary{{
			Departure:    depart,
			Arrival:      depart.Add(25 * time.Minute),
			TimeQuality:  tripplan.TimeFrequencyEstimate,
			Duration:     25 * time.Minute,
			Transfers:    0,
			WalkDistance: 120,
			Legs: []tripplan.Leg{
				{Type: tripplan.LegTransit, Mode: "bus", Line: &tripplan.Line{ID: "test:line:1", ShortName: "T1", LongName: "Contoh A - Contoh B"},
					Headsign: "Contoh B", From: a, To: b, Duration: 23 * time.Minute, Distance: 9000},
				{Type: tripplan.LegWalk, Mode: "walk", From: b, To: end, Duration: 2 * time.Minute, Distance: 120},
			},
		}},
		Attributions: []tripplan.Attribution{{ID: "test-source", Text: "Data uji sintetis", License: "TEST"}},
	}
}

// FakePlanner returns a fixed Result or Err and records each query.
// With Block set, it waits for the context to end and returns its error,
// which simulates a slow routing engine.
type FakePlanner struct {
	Result tripplan.Plan
	Err    error
	Block  bool

	mu      sync.Mutex
	queries []tripplan.Query
}

// Plan implements tripplan.Planner.
func (f *FakePlanner) Plan(ctx context.Context, q tripplan.Query) (tripplan.Plan, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	f.mu.Unlock()
	if f.Block {
		<-ctx.Done()
		return tripplan.Plan{}, ctx.Err()
	}
	return f.Result, f.Err
}

// Queries returns the queries received so far.
func (f *FakePlanner) Queries() []tripplan.Query {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tripplan.Query(nil), f.queries...)
}
