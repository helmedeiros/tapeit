package domain

import (
	"strings"
	"unicode"
)

// Album identity is a domain concern, not an adapter one: an album's coverage
// is measured against its track count, so two spellings of the same record must
// reduce to one identity or the denominator silently changes underneath the
// score. Keeping the rules here — rather than once per adapter — means they
// cannot drift apart and quietly stop agreeing.

// editionMarkers name the qualifiers services append to a record's title for a
// reissue. They mark the same music in different packaging, so they are dropped
// from an album's identity.
var editionMarkers = []string{
	"deluxe", "remaster", "edition", "expanded", "anniversary", "bonus",
	"special", "version", "ultimate", "souvenir", "yearbook", "reissue",
}

// artistSeparators split a credit into its lead act and the rest.
var artistSeparators = []string{",", " & ", " feat", " with "}

// BaseAlbumName reduces an album title to the record it names, dropping edition
// qualifiers and punctuation.
//
// A parenthetical is only removed when it actually contains an edition marker:
// "This Must Be the Place (Naive Melody)" and "(What's the Story) Morning
// Glory?" are titles, not packaging. Merging distinct records is a worse error
// than failing to merge two editions, because the first silently invents
// evidence while the second merely splits it.
func BaseAlbumName(s string) string {
	l := strings.ToLower(s)
	for _, marker := range []string{" (", " [", " - "} {
		if i := strings.Index(l, marker); i > 0 && containsEditionMarker(l[i:]) {
			l = l[:i]
		}
	}
	return squashToAlphanumeric(l, true)
}

// PrimaryArtist reduces a credit to its lead act, so the same record credited
// "Dua Lipa" by one service and "Dua Lipa, DaBaby" by another is one artist.
func PrimaryArtist(s string) string {
	l := strings.ToLower(s)
	for _, sep := range artistSeparators {
		if i := strings.Index(l, sep); i > 0 {
			l = l[:i]
		}
	}
	return squashToAlphanumeric(l, false)
}

// AlbumKey is a stable identity for a record across services and editions. The
// artist is part of the key because album titles are not unique: "Greatest
// Hits" names hundreds of different records.
func AlbumKey(album, artist string) string {
	return BaseAlbumName(album) + "|" + PrimaryArtist(artist)
}

func containsEditionMarker(s string) bool {
	for _, m := range editionMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// squashToAlphanumeric keeps letters and digits, optionally collapsing runs of
// other characters to a single space. Artists drop spaces entirely so that
// "The Black Keys" and "TheBlackKeys" are one act.
func squashToAlphanumeric(s string, keepSpaces bool) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevSpace = false
		case keepSpaces && unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}
