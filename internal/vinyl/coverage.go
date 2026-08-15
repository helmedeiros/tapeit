package vinyl

import (
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/matching"
)

// TracksOnEdition counts how many of a listener's loved tracks are actually on
// a given pressing.
//
// Loved tracks are gathered from years of listening and may span several
// editions of a record, but a listener buys one pressing. Dividing every loved
// track by the standard edition's length therefore overstates what that record
// will deliver — and it overstates it in the direction that matters, telling
// someone they will love a record they only partly love in the form they can
// buy.
//
// Titles are compared on their base form so a qualifier on one side and not the
// other — "Song 2 - 2012 Remaster" against "Song 2", or a "(feat. …)" credit
// the library omits — still recognises the same performance. Matching is
// deliberately anchored rather than substring-based: crediting a record with
// songs it does not contain is the worse error, so "Yellow" must not match
// "Yellow Submarine".
//
// An empty edition listing counts zero. A caller that has not fetched the
// listing should treat that as "not checked" rather than "none present", which
// is why this reports a count and leaves that judgement to the caller.
func TracksOnEdition(loved, edition []string) int {
	if len(loved) == 0 || len(edition) == 0 {
		return 0
	}
	on := make(map[string]struct{}, len(edition))
	for _, t := range edition {
		if k := trackKey(t); k != "" {
			on[k] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(loved))
	count := 0
	for _, t := range loved {
		k := trackKey(t)
		if k == "" {
			continue
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		if _, ok := on[k]; ok {
			count++
		}
	}
	return count
}

// trackKey reduces a track title to the performance it names, dropping the
// version qualifiers services append and disagree about, and folding accents so
// "Señorita" and "Senorita" are one song.
//
// Folding is done here rather than in matching.Normalize because that function
// underpins the push and match pipelines, whose behaviour is settled; widening
// it to serve this comparison would risk those for no benefit to them.
func trackKey(title string) string {
	t := title
	for _, sep := range []string{" - ", " (", " ["} {
		if i := strings.Index(t, sep); i > 0 {
			t = t[:i]
		}
	}
	return matching.Normalize(foldDiacritics(t))
}

// latinFolds maps the accented Latin letters that appear in track titles to
// their plain forms. The project carries no dependencies, and a table this size
// is a smaller cost than pulling in Unicode normalisation for one comparison.
var latinFolds = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ä': 'a', 'ã': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'ö': 'o', 'õ': 'o', 'ø': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ñ': 'n', 'ç': 'c', 'ý': 'y', 'ÿ': 'y', 'š': 's', 'ž': 'z',
}

// foldDiacritics replaces accented Latin letters with their plain forms.
func foldDiacritics(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if folded, ok := latinFolds[r]; ok {
			b.WriteRune(folded)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// PressingLookup returns the track titles of the pressing a shortlisted record
// would be bought as. The bool reports whether the listing could be read at all,
// which is not the same as it being empty: a pressing we could not fetch is
// unknown, and treating it as holding nothing would zero a good record's
// coverage on a network failure.
type PressingLookup func(Scored) ([]string, bool)

// Refine re-scores a shortlist against the pressings it would actually be
// bought as, and re-orders it on the result.
//
// This is deliberately a second pass over a shortlist rather than part of
// ranking. Reading a track listing costs a request per record, which is worth
// spending on twenty candidates and not on five hundred — and until a record is
// a candidate, the optimistic across-editions count is good enough to rank it.
//
// Verification can change the order, not merely the numbers, which is the whole
// reason to do it: a record whose loved tracks turn out to be deluxe-only
// delivers less than one whose tracks are all on the standard pressing.
func Refine(ranked []Scored, lookup PressingLookup, o Options) []Scored {
	maxRW := 0.0
	for _, s := range ranked {
		if s.EstimatedPlays > maxRW {
			maxRW = s.EstimatedPlays
		}
	}
	out := make([]Scored, 0, len(ranked))
	for _, s := range ranked {
		titles, ok := lookup(s)
		if !ok {
			out = append(out, s)
			continue
		}
		e := s.Evidence
		e.PressingTracks, e.PressingChecked = TracksOnEdition(e.LovedTitles, titles), true
		out = append(out, Score(e, maxRW, o))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Album < out[j].Album
	})
	return out
}
