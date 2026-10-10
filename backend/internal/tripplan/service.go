package tripplan

import (
	"context"
	"errors"
	"fmt"
)

// Service validates queries, calls the Planner and checks its results.
type Service struct {
	planner  Planner
	walkOnly WalkOnlyPolicy
}

// WalkOnlyPolicy decides whether an itinerary without any transit leg may be
// returned. Walking access and egress legs of transit itineraries are not
// affected.
type WalkOnlyPolicy func(Itinerary) bool

// RejectWalkOnly is the default policy. No maximum walking distance has been
// approved, and the API contract cannot yet tell the client "just walk", so
// walk-only itineraries are dropped rather than judged by an invented
// threshold. If nothing else remains, the result is ErrNoRoute.
func RejectWalkOnly(Itinerary) bool { return false }

// Option configures a Service.
type Option func(*Service)

// WithWalkOnlyPolicy replaces the default RejectWalkOnly policy.
func WithWalkOnlyPolicy(p WalkOnlyPolicy) Option {
	return func(s *Service) {
		if p != nil {
			s.walkOnly = p
		}
	}
}

// NewService returns a Service. A nil planner is allowed: every Plan call
// then fails with ErrRoutingUnavailable.
func NewService(p Planner, opts ...Option) *Service {
	s := &Service{planner: p, walkOnly: RejectWalkOnly}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Plan returns at most q.MaxItineraries itineraries. After validating the
// planner's output it drops walk-only itineraries the policy rejects, merges
// structurally identical itineraries (see Deduplicate), then truncates.
func (s *Service) Plan(ctx context.Context, q Query) (Plan, error) {
	if err := q.Validate(); err != nil {
		return Plan{}, err
	}
	if s == nil || s.planner == nil {
		return Plan{}, ErrRoutingUnavailable
	}

	p, err := s.planner.Plan(ctx, q)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return Plan{}, ErrRoutingTimeout
		}
		return Plan{}, err
	}

	if len(p.Itineraries) == 0 {
		return Plan{}, ErrNoRoute
	}
	if err := checkPlan(p); err != nil {
		return Plan{}, fmt.Errorf("%w: %v", ErrUpstreamInvalid, err)
	}

	kept := make([]Itinerary, 0, len(p.Itineraries))
	for _, it := range p.Itineraries {
		if hasTransit(it) || s.walkOnly(it) {
			kept = append(kept, it)
		}
	}
	if len(kept) == 0 {
		return Plan{}, ErrNoRoute
	}
	p.Itineraries = Deduplicate(kept)

	if len(p.Itineraries) > q.MaxItineraries {
		p.Itineraries = p.Itineraries[:q.MaxItineraries]
	}
	return p, nil
}

func hasTransit(it Itinerary) bool {
	for _, l := range it.Legs {
		if l.Type == LegTransit {
			return true
		}
	}
	return false
}

// checkPlan rejects planner output that the API must not pass on.
// Results without attribution are rejected because the data licences
// (CC BY 4.0, ODbL) require it.
func checkPlan(p Plan) error {
	if len(p.Attributions) == 0 {
		return errors.New("missing attributions")
	}
	for i, a := range p.Attributions {
		if a.ID == "" || a.Text == "" {
			return fmt.Errorf("attribution %d incomplete", i)
		}
	}
	for i, it := range p.Itineraries {
		if len(it.Legs) == 0 {
			return fmt.Errorf("itinerary %d has no legs", i)
		}
		if it.Departure.IsZero() || it.Arrival.IsZero() || it.Arrival.Before(it.Departure) {
			return fmt.Errorf("itinerary %d has invalid times", i)
		}
		if it.TimeQuality != TimeScheduled && it.TimeQuality != TimeFrequencyEstimate {
			return fmt.Errorf("itinerary %d has invalid time quality", i)
		}
		if it.Duration < 0 || it.Transfers < 0 || it.WalkDistance < 0 {
			return fmt.Errorf("itinerary %d has negative values", i)
		}
		for j, l := range it.Legs {
			if err := checkLeg(l); err != nil {
				return fmt.Errorf("itinerary %d leg %d: %w", i, j, err)
			}
		}
	}
	return nil
}

func checkLeg(l Leg) error {
	switch l.Type {
	case LegTransit:
		if l.Line == nil || l.Line.ID == "" {
			return errors.New("transit leg without line")
		}
	case LegWalk:
	default:
		return errors.New("unknown leg type")
	}
	if l.Mode == "" || l.Duration < 0 || l.Distance < 0 {
		return errors.New("invalid mode or negative values")
	}
	if !l.From.Coordinate.Valid() || !l.To.Coordinate.Valid() {
		return errors.New("coordinate out of range")
	}
	return nil
}
