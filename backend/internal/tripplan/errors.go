package tripplan

import (
	"errors"
	"strings"
)

// Errors returned by Service.Plan and expected from Planner implementations.
// Planners may wrap them; callers match with errors.Is / errors.As.
var (
	ErrNoRoute            = errors.New("no route found")
	ErrRoutingUnavailable = errors.New("routing unavailable")
	ErrRoutingTimeout     = errors.New("routing timed out")
	ErrUpstreamInvalid    = errors.New("invalid routing response")
)

// Side names which endpoint a location error refers to.
type Side string

const (
	SideOrigin      Side = "origin"
	SideDestination Side = "destination"
	SideUnknown     Side = ""
)

// OutOfCoverageError means an endpoint lies outside the service area.
type OutOfCoverageError struct{ Side Side }

func (e *OutOfCoverageError) Error() string { return "location outside coverage" }

// LocationNotRoutableError means an endpoint is inside the service area but
// cannot be connected to the routing network.
type LocationNotRoutableError struct{ Side Side }

func (e *LocationNotRoutableError) Error() string { return "location not routable" }

// ValidationError lists invalid query fields.
type ValidationError struct{ Problems []Problem }

func (e *ValidationError) Error() string {
	fields := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		fields[i] = p.Field + ":" + p.Reason
	}
	return "invalid query: " + strings.Join(fields, ", ")
}
