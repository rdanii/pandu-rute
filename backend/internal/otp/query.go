package otp

import (
	"encoding/json"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

// planQuery uses only fields present in the OTP 2.10.0 GTFS GraphQL schema
// (see docs/routing-data-evaluation.md, "OTP 2.10 API behaviour"):
// planConnection arguments origin, destination, dateTime.earliestDeparture
// and first; RoutingError.inputField; LegTime scheduledTime. Labels are not
// sent upstream.
const planQuery = `query PanduRutePlan($origin: PlanLabeledLocationInput!, $destination: PlanLabeledLocationInput!, $departAt: OffsetDateTime!, $first: Int) {
  planConnection(origin: $origin, destination: $destination, dateTime: {earliestDeparture: $departAt}, first: $first) {
    routingErrors { code inputField }
    edges {
      node {
        start
        end
        duration
        numberOfTransfers
        walkDistance
        legs {
          mode
          transitLeg
          duration
          distance
          headsign
          start { scheduledTime }
          end { scheduledTime }
          from { name lat lon stop { gtfsId } }
          to { name lat lon stop { gtfsId } }
          route { gtfsId shortName longName mode }
          trip { gtfsId }
        }
      }
    }
  }
}`

type graphQLRequest struct {
	Query     string        `json:"query"`
	Variables planVariables `json:"variables"`
}

type coordinateInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type locationInput struct {
	Coordinate coordinateInput `json:"coordinate"`
}

type labeledLocationInput struct {
	Location locationInput `json:"location"`
}

type planVariables struct {
	Origin      labeledLocationInput `json:"origin"`
	Destination labeledLocationInput `json:"destination"`
	DepartAt    string               `json:"departAt"`
	First       int                  `json:"first"`
}

func newPlanVariables(q tripplan.Query) planVariables {
	loc := func(c tripplan.Coordinate) labeledLocationInput {
		return labeledLocationInput{Location: locationInput{Coordinate: coordinateInput{Latitude: c.Lat, Longitude: c.Lon}}}
	}
	return planVariables{
		Origin:      loc(q.Origin.Coordinate),
		Destination: loc(q.Destination.Coordinate),
		DepartAt:    q.DepartAt.Format(time.RFC3339),
		First:       q.MaxItineraries,
	}
}

// Response structs. Pointers mark fields whose absence must be detected.

// graphQLResponse keeps GraphQL error entries opaque: their text is never read.
type graphQLResponse struct {
	Data   *responseData     `json:"data"`
	Errors []json.RawMessage `json:"errors"`
}

type responseData struct {
	PlanConnection *planConnection `json:"planConnection"`
}

type planConnection struct {
	RoutingErrors []routingError `json:"routingErrors"`
	Edges         []*planEdge    `json:"edges"`
}

type routingError struct {
	Code       string  `json:"code"`
	InputField *string `json:"inputField"`
}

type planEdge struct {
	Node *itinerary `json:"node"`
}

type itinerary struct {
	Start             *string  `json:"start"`
	End               *string  `json:"end"`
	Duration          *int64   `json:"duration"`
	NumberOfTransfers *int     `json:"numberOfTransfers"`
	WalkDistance      *float64 `json:"walkDistance"`
	Legs              []*leg   `json:"legs"`
}

type legTime struct {
	ScheduledTime *string `json:"scheduledTime"`
}

type stopRef struct {
	GtfsID *string `json:"gtfsId"`
}

type place struct {
	Name *string  `json:"name"`
	Lat  *float64 `json:"lat"`
	Lon  *float64 `json:"lon"`
	Stop *stopRef `json:"stop"`
}

type routeRef struct {
	GtfsID    *string `json:"gtfsId"`
	ShortName *string `json:"shortName"`
	LongName  *string `json:"longName"`
	Mode      *string `json:"mode"`
}

type tripRef struct {
	GtfsID *string `json:"gtfsId"`
}

type leg struct {
	Mode       *string   `json:"mode"`
	TransitLeg *bool     `json:"transitLeg"`
	Duration   *float64  `json:"duration"`
	Distance   *float64  `json:"distance"`
	Headsign   *string   `json:"headsign"`
	Start      *legTime  `json:"start"`
	End        *legTime  `json:"end"`
	From       *place    `json:"from"`
	To         *place    `json:"to"`
	Route      *routeRef `json:"route"`
	Trip       *tripRef  `json:"trip"`
}
