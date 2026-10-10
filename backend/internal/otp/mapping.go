package otp

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

// Routing error codes verified in the OTP 2.10.0 RoutingErrorCode enum.
const (
	codeLocationNotFound                  = "LOCATION_NOT_FOUND"
	codeOutsideBounds                     = "OUTSIDE_BOUNDS"
	codeNoStopsInRange                    = "NO_STOPS_IN_RANGE"
	codeNoTransitConnection               = "NO_TRANSIT_CONNECTION"
	codeNoTransitConnectionInSearchWindow = "NO_TRANSIT_CONNECTION_IN_SEARCH_WINDOW"
	codeNoDirectModeConnection            = "NO_DIRECT_MODE_CONNECTION"
	codeOutsideServicePeriod              = "OUTSIDE_SERVICE_PERIOD"
	codeWalkingBetterThanTransit          = "WALKING_BETTER_THAN_TRANSIT"
)

// routingErrorToDomain maps OTP routing errors. OTP may return walk-only
// itineraries together with a routing error (observed for
// OUTSIDE_SERVICE_PERIOD and WALKING_BETTER_THAN_TRANSIT), so any routing
// error wins over returned itineraries.
//
// Location errors are never reported as OUT_OF_COVERAGE: that requires the
// app's own service-area check, which does not exist yet. The side comes
// only from the structured inputField, never from description text.
func routingErrorToDomain(errs []routingError) error {
	for _, e := range errs {
		if e.Code == codeLocationNotFound || e.Code == codeOutsideBounds {
			return &tripplan.LocationNotRoutableError{Side: side(e.InputField)}
		}
	}
	// NO_STOPS_IN_RANGE, NO_TRANSIT_CONNECTION*, NO_DIRECT_MODE_CONNECTION,
	// OUTSIDE_SERVICE_PERIOD, WALKING_BETTER_THAN_TRANSIT and unknown codes.
	//
	// The public contract has no code for "date outside the loaded service
	// period" or "walking is better", so these become NO_ROUTE. That is safe
	// (nothing is shown) but less specific than OTP's answer. Error
	// precedence may also hide transit itineraries OTP returned alongside
	// e.g. WALKING_BETTER_THAN_TRANSIT; this is deliberate until the contract
	// can express it.
	return tripplan.ErrNoRoute
}

func side(inputField *string) tripplan.Side {
	if inputField == nil {
		return tripplan.SideUnknown
	}
	switch *inputField {
	case "FROM":
		return tripplan.SideOrigin
	case "TO":
		return tripplan.SideDestination
	}
	return tripplan.SideUnknown
}

func (c *Client) toPlan(q tripplan.Query, pc *planConnection) (tripplan.Plan, error) {
	if len(pc.RoutingErrors) > 0 {
		return tripplan.Plan{}, routingErrorToDomain(pc.RoutingErrors)
	}
	its := make([]tripplan.Itinerary, 0, len(pc.Edges))
	for i, e := range pc.Edges {
		if e == nil || e.Node == nil {
			return tripplan.Plan{}, invalid("edge %d has no node", i)
		}
		it, err := c.toItinerary(q, e.Node)
		if err != nil {
			return tripplan.Plan{}, fmt.Errorf("%w: itinerary %d: %v", tripplan.ErrUpstreamInvalid, i, err)
		}
		its = append(its, it)
	}
	if len(its) == 0 {
		return tripplan.Plan{}, tripplan.ErrNoRoute
	}
	return tripplan.Plan{
		Itineraries:  its,
		Attributions: append([]tripplan.Attribution(nil), c.attributions...),
		// DataVersion stays nil: OTP's response does not carry dataset
		// versions, and they are never invented.
	}, nil
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{tripplan.ErrUpstreamInvalid}, args...)...)
}

func (c *Client) toItinerary(q tripplan.Query, n *itinerary) (tripplan.Itinerary, error) {
	if len(n.Legs) == 0 {
		return tripplan.Itinerary{}, errors.New("no legs")
	}
	if n.NumberOfTransfers == nil {
		return tripplan.Itinerary{}, errors.New("missing numberOfTransfers")
	}
	legs := make([]tripplan.Leg, 0, len(n.Legs))
	var walk float64
	var legStart, legEnd time.Time
	for i, l := range n.Legs {
		if l == nil {
			return tripplan.Itinerary{}, fmt.Errorf("leg %d missing", i)
		}
		dl, start, end, err := c.toLeg(l, i == 0, i == len(n.Legs)-1, q)
		if err != nil {
			return tripplan.Itinerary{}, fmt.Errorf("leg %d: %w", i, err)
		}
		if i == 0 {
			legStart = start
		}
		legEnd = end
		if dl.Type == tripplan.LegWalk {
			walk += dl.Distance
		}
		legs = append(legs, dl)
	}

	dep, err := optionalTime(n.Start, legStart)
	if err != nil {
		return tripplan.Itinerary{}, fmt.Errorf("start: %w", err)
	}
	arr, err := optionalTime(n.End, legEnd)
	if err != nil {
		return tripplan.Itinerary{}, fmt.Errorf("end: %w", err)
	}
	dur := arr.Sub(dep)
	if n.Duration != nil {
		dur = time.Duration(*n.Duration) * time.Second
	}
	if n.WalkDistance != nil {
		walk = *n.WalkDistance
	}
	return tripplan.Itinerary{
		Departure:    dep,
		Arrival:      arr,
		TimeQuality:  c.timeQuality,
		Duration:     dur,
		Transfers:    *n.NumberOfTransfers,
		WalkDistance: walk,
		Legs:         legs,
	}, nil
}

func (c *Client) toLeg(l *leg, first, last bool, q tripplan.Query) (tripplan.Leg, time.Time, time.Time, error) {
	var zero time.Time
	if l.Mode == nil || l.TransitLeg == nil || l.Duration == nil || l.Distance == nil {
		return tripplan.Leg{}, zero, zero, errors.New("missing mode, transitLeg, duration or distance")
	}
	start, err := legTimeValue(l.Start)
	if err != nil {
		return tripplan.Leg{}, zero, zero, fmt.Errorf("start: %w", err)
	}
	end, err := legTimeValue(l.End)
	if err != nil {
		return tripplan.Leg{}, zero, zero, fmt.Errorf("end: %w", err)
	}
	from, err := c.toPlace(l.From, first, q.Origin.Label)
	if err != nil {
		return tripplan.Leg{}, zero, zero, fmt.Errorf("from: %w", err)
	}
	to, err := c.toPlace(l.To, last, q.Destination.Label)
	if err != nil {
		return tripplan.Leg{}, zero, zero, fmt.Errorf("to: %w", err)
	}

	out := tripplan.Leg{
		From:     from,
		To:       to,
		Duration: time.Duration(*l.Duration * float64(time.Second)),
		Distance: *l.Distance,
	}
	switch {
	case *l.TransitLeg:
		if l.Route == nil || l.Route.GtfsID == nil {
			return tripplan.Leg{}, zero, zero, errors.New("transit leg without route")
		}
		id, err := c.localID(*l.Route.GtfsID)
		if err != nil {
			return tripplan.Leg{}, zero, zero, fmt.Errorf("route: %w", err)
		}
		out.Type = tripplan.LegTransit
		out.Mode = strings.ToLower(*l.Mode)
		out.Line = &tripplan.Line{
			ID:        c.namespace + ":line:" + id,
			ShortName: deref(l.Route.ShortName),
			LongName:  deref(l.Route.LongName),
		}
		out.Headsign = deref(l.Headsign)
	case *l.Mode == "WALK":
		out.Type = tripplan.LegWalk
		out.Mode = "walk"
	default:
		// Only walking and transit are requested; anything else is unexpected.
		return tripplan.Leg{}, zero, zero, errors.New("unexpected non-transit mode")
	}
	return out, start, end, nil
}

// toPlace maps a leg endpoint. For non-stop places OTP returns placeholder
// names ("Origin", "Destination"); those are replaced by the client's own
// label at the trip ends, or left empty.
func (c *Client) toPlace(p *place, tripEnd bool, label string) (tripplan.Place, error) {
	if p == nil || p.Lat == nil || p.Lon == nil {
		return tripplan.Place{}, errors.New("missing place or coordinates")
	}
	out := tripplan.Place{Coordinate: tripplan.Coordinate{Lat: *p.Lat, Lon: *p.Lon}}
	if p.Stop != nil {
		if p.Stop.GtfsID == nil {
			return tripplan.Place{}, errors.New("stop without gtfsId")
		}
		id, err := c.localID(*p.Stop.GtfsID)
		if err != nil {
			return tripplan.Place{}, fmt.Errorf("stop: %w", err)
		}
		out.StopID = c.namespace + ":stop:" + id
		out.Name = deref(p.Name)
		return out, nil
	}
	if tripEnd {
		out.Name = label
	}
	return out, nil
}

// localID strips the configured OTP feed prefix ("1:G00012" -> "G00012").
func (c *Client) localID(gtfsID string) (string, error) {
	id, ok := strings.CutPrefix(gtfsID, c.feedPrefix)
	if !ok || id == "" {
		return "", errors.New("unexpected feed id")
	}
	return id, nil
}

func legTimeValue(t *legTime) (time.Time, error) {
	if t == nil || t.ScheduledTime == nil {
		return time.Time{}, errors.New("missing scheduledTime")
	}
	return parseOffsetDateTime(*t.ScheduledTime)
}

func optionalTime(s *string, fallback time.Time) (time.Time, error) {
	if s == nil {
		return fallback, nil
	}
	return parseOffsetDateTime(*s)
}

// parseOffsetDateTime accepts OTP's ISO-8601 OffsetDateTime with or without
// seconds (the schema's own example omits them; responses include them).
func parseOffsetDateTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("invalid OffsetDateTime")
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
