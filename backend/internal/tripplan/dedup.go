package tripplan

import (
	"strconv"
	"strings"
)

// Deduplicate removes itineraries that describe the same journey structure,
// such as alternatives that differ only by a departure shifted by a minute
// (observed with OTP on the frequency-based TransJakarta feed; the cause is
// not proven).
//
// The structure key is the ordered list of legs:
//
//	transit: "T", line ID, boarding stop ID, alighting stop ID
//	walk:    "W", from stop ID or "-", to stop ID or "-"
//
// Departure times, durations, distances, names and headsigns are not part of
// the key. Two runs of the same line between the same stops collapse even if
// they are different trip variants; the representative keeps its own times
// and headsign.
//
// Itineraries whose structure cannot be keyed reliably (a transit leg without
// a line ID or stop IDs) are never merged.
//
// Within a group the representative is the earliest arrival, then fewer
// transfers, then shorter walking distance, then the earlier position in the
// input. Groups keep the order of their first appearance.
func Deduplicate(its []Itinerary) []Itinerary {
	out := make([]Itinerary, 0, len(its))
	index := make(map[string]int, len(its)) // key -> position in out
	for _, it := range its {
		key, ok := structureKey(it)
		if !ok {
			out = append(out, it)
			continue
		}
		if i, seen := index[key]; seen {
			if better(it, out[i]) {
				out[i] = it
			}
			continue
		}
		index[key] = len(out)
		out = append(out, it)
	}
	return out
}

func structureKey(it Itinerary) (string, bool) {
	if len(it.Legs) == 0 {
		return "", false
	}
	var b strings.Builder
	for _, l := range it.Legs {
		switch l.Type {
		case LegTransit:
			if l.Line == nil || l.Line.ID == "" || l.From.StopID == "" || l.To.StopID == "" {
				return "", false
			}
			writeKeyPart(&b, "T", l.Line.ID, l.From.StopID, l.To.StopID)
		case LegWalk:
			writeKeyPart(&b, "W", anchor(l.From), anchor(l.To))
		default:
			return "", false
		}
	}
	return b.String(), true
}

func anchor(p Place) string {
	if p.StopID == "" {
		return "-"
	}
	return p.StopID
}

// writeKeyPart length-prefixes each field so ids containing separators
// cannot collide.
func writeKeyPart(b *strings.Builder, fields ...string) {
	for _, f := range fields {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	b.WriteByte('|')
}

// better reports whether a should replace b as a group's representative.
func better(a, b Itinerary) bool {
	if !a.Arrival.Equal(b.Arrival) {
		return a.Arrival.Before(b.Arrival)
	}
	if a.Transfers != b.Transfers {
		return a.Transfers < b.Transfers
	}
	return a.WalkDistance < b.WalkDistance
}
