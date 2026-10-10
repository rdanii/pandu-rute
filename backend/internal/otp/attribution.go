package otp

import (
	"pandu-rute/backend/internal/attribution"
	"pandu-rute/backend/internal/tripplan"
)

// PilotAttributions returns the credits required for the pilot graph built
// from the TransJakarta GTFS Static feed and OpenStreetMap.
func PilotAttributions() []tripplan.Attribution {
	return []tripplan.Attribution{
		attribution.TransJakartaGTFS(),
		attribution.OpenStreetMap(),
	}
}
