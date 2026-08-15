package vinyl

import "sort"

// Edition is one released version of a record: standard, deluxe, anniversary.
// They are the same music in different packaging, and which one is chosen sets
// the denominator of coverage — so the choice is part of the model, not a
// detail of fetching.
type Edition struct {
	ID            string
	Name          string
	Artist        string
	TrackCount    int
	RuntimeMin    int
	UPC           string
	Genres        []string
	IsCompilation bool
}

// ChooseEdition picks which edition of a record to judge a listener by.
//
// Two rules, in order:
//
//   - An edition must be able to hold what was actually heard. The observed
//     count of loved tracks is a hard lower bound: you cannot love twelve tracks
//     from a ten-track record. Choosing a smaller edition would overstate
//     coverage, and — because a too-small edition can fall below the minimum
//     track count that separates an album from an EP — could drop the record
//     from consideration entirely without ever reporting it.
//   - Among the editions that fit, the smallest wins. A deluxe edition's padding
//     understates how much of the record the listener loves, and the standard
//     pressing is what one actually buys on vinyl.
//
// When no edition can hold the observed count — which happens when a listener's
// loved tracks span several editions — the largest is the closest honest
// denominator. Returning nothing would silently drop a record the listener
// demonstrably lives with.
//
// Editions with no known track count carry no denominator and are never chosen.
func ChooseEdition(eds []Edition, observedLoved int) (Edition, bool) {
	usable := make([]Edition, 0, len(eds))
	for _, e := range eds {
		if e.TrackCount > 0 {
			usable = append(usable, e)
		}
	}
	if len(usable) == 0 {
		return Edition{}, false
	}
	// Sort by track count, then id, so equal-sized editions resolve the same way
	// on every run rather than by map iteration order.
	sort.Slice(usable, func(i, j int) bool {
		if usable[i].TrackCount != usable[j].TrackCount {
			return usable[i].TrackCount < usable[j].TrackCount
		}
		return usable[i].ID < usable[j].ID
	})
	for _, e := range usable {
		if e.TrackCount >= observedLoved {
			return e, true
		}
	}
	return usable[len(usable)-1], true
}
