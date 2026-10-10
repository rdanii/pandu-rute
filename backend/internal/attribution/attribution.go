// Package attribution holds the data-source credits the API must show.
package attribution

import "pandu-rute/backend/internal/tripplan"

// TransJakartaGTFS credits the TransJakarta GTFS Static feed:
// "GTFS Static Transjakarta © 2026 oleh PT Transportasi Jakarta
// dilisensikan di bawah CC BY 4.0." (https://ppid.transjakarta.co.id/informasi/berkala).
func TransJakartaGTFS() tripplan.Attribution {
	return tripplan.Attribution{
		ID:         "transjakarta-gtfs",
		Text:       "GTFS Static Transjakarta © 2026 PT Transportasi Jakarta, dilisensikan di bawah CC BY 4.0. Diolah oleh Pandu Rute.",
		License:    "CC-BY-4.0",
		LicenseURL: "https://creativecommons.org/licenses/by/4.0/",
		SourceURL:  "https://ppid.transjakarta.co.id/informasi/berkala",
	}
}

// OpenStreetMap credits OpenStreetMap data (ODbL, "© OpenStreetMap
// contributors", https://www.openstreetmap.org/copyright).
func OpenStreetMap() tripplan.Attribution {
	return tripplan.Attribution{
		ID:         "openstreetmap",
		Text:       "© Kontributor OpenStreetMap",
		License:    "ODbL-1.0",
		LicenseURL: "https://www.openstreetmap.org/copyright",
		SourceURL:  "https://www.openstreetmap.org/",
	}
}
