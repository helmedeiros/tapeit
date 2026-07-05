package curator

import "strings"

// versionMarkers flag a title suffix as a re-release or variant rather than a
// distinct song, so "Come Together - Remastered 2009" collapses onto
// "Come Together". Kept conservative to avoid merging genuinely different songs.
var versionMarkers = []string{
	"remaster", "remastered", "mono", "stereo",
	"radio edit", "single version", "album version", "extended",
	"anniversary", "deluxe", "bonus", "reissue",
	"live", "acoustic", "demo", "session",
	"feat", "featuring", "explicit", "reprise",
}

// dashMarkers are additionally treated as variants after a " - " separator,
// where a bare descriptor almost always means a re-release, not part of a title.
var dashMarkers = []string{"mix", "remix", "edit", "version", "take"}

// baseTitle strips a trailing version descriptor from a track title, yielding the
// canonical song title used to collapse near-duplicate releases.
func baseTitle(title string) string {
	out := strings.TrimSpace(title)
	for {
		stripped := strings.TrimSpace(stripVersionParens(stripDashTail(out)))
		if stripped == out || stripped == "" {
			return out
		}
		out = stripped
	}
}

// isClean reports whether a title carries no version descriptor.
func isClean(title string) bool {
	return baseTitle(title) == strings.TrimSpace(title)
}

func stripDashTail(s string) string {
	i := strings.LastIndex(s, " - ")
	if i < 0 {
		return s
	}
	tail := s[i+3:]
	if containsMarker(tail, versionMarkers) || containsMarker(tail, dashMarkers) {
		return s[:i]
	}
	return s
}

func stripVersionParens(s string) string {
	s = strings.TrimSpace(s)
	var open byte
	switch {
	case strings.HasSuffix(s, ")"):
		open = '('
	case strings.HasSuffix(s, "]"):
		open = '['
	default:
		return s
	}
	j := strings.LastIndexByte(s, open)
	if j < 0 {
		return s
	}
	if containsMarker(s[j+1:len(s)-1], versionMarkers) {
		return s[:j]
	}
	return s
}

// containsMarker matches a marker as a token prefix (so "remaster" catches
// "remastered") or, for multi-word markers, as consecutive tokens — avoiding
// false hits like "live" inside "olive".
func containsMarker(s string, markers []string) bool {
	tokens := strings.Fields(strings.ToLower(s))
	joined := " " + strings.Join(tokens, " ") + " "
	for _, m := range markers {
		if strings.Contains(m, " ") {
			if strings.Contains(joined, " "+m+" ") {
				return true
			}
			continue
		}
		for _, tok := range tokens {
			if strings.HasPrefix(tok, m) {
				return true
			}
		}
	}
	return false
}
