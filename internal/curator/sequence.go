package curator

import (
	"sort"

	"github.com/helmedeiros/tapeit/internal/matching"
)

// Flow is the tempo shape a sequence should follow.
type Flow int

const (
	// FlowNone keeps only the no-adjacent-same-artist ordering (tempo ignored).
	FlowNone Flow = iota
	// FlowSmooth ramps tempo up steadily from the slowest track to the fastest.
	FlowSmooth
	// FlowArc rises to a tempo peak in the middle, then eases back down.
	FlowArc
)

// ParseFlow maps a flag value to a Flow, reporting whether it was recognised.
func ParseFlow(s string) (Flow, bool) {
	switch s {
	case "", "none":
		return FlowNone, true
	case "smooth", "ascending":
		return FlowSmooth, true
	case "arc":
		return FlowArc, true
	default:
		return FlowNone, false
	}
}

// Sequence orders tracks by tempo (per flow) while keeping the invariant that no
// two adjacent tracks share an artist. Tracks without a BPM can't be placed on
// the tempo curve, so they're separated among themselves and appended after the
// tempo-ordered run. With too little BPM signal it falls back to plain artist
// separation.
func Sequence(tracks []Track, flow Flow) []Track {
	if flow == FlowNone {
		return separate(tracks)
	}
	var known, unknown []Track
	for _, t := range tracks {
		if t.BPM > 0 {
			known = append(known, t)
		} else {
			unknown = append(unknown, t)
		}
	}
	if len(known) < 2 {
		return separate(tracks)
	}
	sort.SliceStable(known, func(i, j int) bool {
		if known[i].BPM != known[j].BPM {
			return known[i].BPM < known[j].BPM
		}
		return known[i].Title < known[j].Title
	})
	if flow == FlowArc {
		known = arc(known)
	}
	ordered := repairAdjacent(known)
	return append(ordered, separate(unknown)...)
}

// arc rearranges a tempo-ascending slice into a bitonic curve: tempo climbs to a
// peak near the middle and descends after it (…a, c, e, d, b for a<b<c<d<e).
func arc(sorted []Track) []Track {
	out := make([]Track, len(sorted))
	left, right := 0, len(out)-1
	for i, t := range sorted {
		if i%2 == 0 {
			out[left] = t
			left++
		} else {
			out[right] = t
			right--
		}
	}
	return out
}

// repairAdjacent breaks same-artist adjacencies by swapping the offending track
// with the nearest later track of a different artist — a local fix that leaves
// the surrounding tempo order intact.
func repairAdjacent(seq []Track) []Track {
	out := append([]Track(nil), seq...)
	for i := 1; i < len(out); i++ {
		if matching.Normalize(out[i].Artist) != matching.Normalize(out[i-1].Artist) {
			continue
		}
		for j := i + 1; j < len(out); j++ {
			if matching.Normalize(out[j].Artist) != matching.Normalize(out[i-1].Artist) {
				out[i], out[j] = out[j], out[i]
				break
			}
		}
	}
	return out
}
