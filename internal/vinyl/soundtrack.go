package vinyl

import "strings"

// Soundtracks are excluded by default not because they are lesser records, but
// because they are usually shared or incidental listening — a household, a car,
// a film seen years ago — rather than the deliberate solo listening a record is
// bought for. That makes the judgement consequential: an excluded record does
// not merely rank lower, it never appears.

// soundtrackGenres are the genre names Apple files accompanying music under.
//
// The catalog localises genre names per storefront, so this list is language
// dependent: these are the forms observed on the English and German
// storefronts, where a soundtrack comes back as "Soundtracks" or "Filmmusik".
// Other storefronts will use other words, and adding them is a matter of
// observing what they actually return — guessing at translations would put
// unverified data in the path of a decision, which is worse than a gap that
// falls back to reading the title.
var soundtrackGenres = []string{
	"soundtrack", "filmmusik", "musicals", "original score", "score",
}

// explicitSoundtrackTitles are the phrases that state outright what a record
// accompanies. They are deliberately the unambiguous forms only: a title is
// weak evidence, and convicting a record on a phrase like "music from the" or a
// bare "(original" would exclude "Music from Big Pink" and "Original Pirate
// Material" — real albums, silently removed from consideration.
var explicitSoundtrackTitles = []string{
	"original motion picture soundtrack",
	"original motion picture score",
	"motion picture soundtrack",
	"original soundtrack",
	"original series soundtrack",
	"original television soundtrack",
	"original cast recording",
	"original broadway cast",
	"music from the motion picture",
	"songs from the motion picture",
}

// IsSoundtrack reports whether a record is music made to accompany something
// else.
//
// Genres are the catalog's own answer and are trusted first. A title is
// consulted only when the catalog gave no genres — which happens for albums that
// could not be resolved — and then only for phrases that name a film or a
// production outright.
func IsSoundtrack(album, artist string, genres []string) bool {
	for _, g := range genres {
		l := strings.ToLower(g)
		for _, m := range soundtrackGenres {
			if strings.Contains(l, m) {
				return true
			}
		}
	}
	if len(genres) > 0 {
		// The catalog answered and did not say soundtrack. Its answer stands.
		return false
	}
	l := strings.ToLower(album + " " + artist)
	for _, m := range explicitSoundtrackTitles {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
