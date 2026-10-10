package otp

import "pandu-rute/backend/internal/tripplan"

// PilotAttributions returns the credits required for the pilot graph built
// from the TransJakarta GTFS Static feed and OpenStreetMap.
//
// TransJakarta: "GTFS Static Transjakarta © 2026 oleh PT Transportasi Jakarta
// dilisensikan di bawah CC BY 4.0." (https://ppid.transjakarta.co.id/informasi/berkala).
// OpenStreetMap: ODbL, credit "© OpenStreetMap contributors"
// (https://www.openstreetmap.org/copyright).
func PilotAttributions() []tripplan.Attribution {
	return []tripplan.Attribution{
		{
			ID:         "transjakarta-gtfs",
			Text:       "GTFS Static Transjakarta © 2026 PT Transportasi Jakarta, dilisensikan di bawah CC BY 4.0. Diolah oleh Pandu Rute.",
			License:    "CC-BY-4.0",
			LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
			SourceURL:  "https://ppid.transjakarta.co.id/informasi/berkala",
		},
		{
			ID:         "openstreetmap",
			Text:       "© Kontributor OpenStreetMap",
			License:    "ODbL-1.0",
			LicenseURL: "https://www.openstreetmap.org/copyright",
			SourceURL:  "https://www.openstreetmap.org/",
		},
	}
}
