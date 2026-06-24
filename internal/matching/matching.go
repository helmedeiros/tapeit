// Package matching resolves source tracks to target-catalog songs. It is an
// application service: it depends only on the domain.CatalogPort, never on a
// concrete music provider.
package matching

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// isrcBatch is how many ISRCs to request per catalog call. Apple returns at
// most 25 songs per response and one ISRC can expand to several songs, so we
// keep the batch well below 25.
const isrcBatch = 15

// durationToleranceMS is how far a candidate's duration may differ from the
// source track while still counting as the same recording.
const durationToleranceMS = 2500

// searchThrottle paces text-search calls; Apple rate-limits this endpoint more
// aggressively than ISRC lookups.
const searchThrottle = 250 * time.Millisecond

// Service turns tracks into matches using a catalog.
type Service struct {
	catalog  domain.CatalogPort
	progress func(string)
}

// New builds a matching service. progress may be nil.
func New(catalog domain.CatalogPort, progress func(string)) *Service {
	return &Service{catalog: catalog, progress: progress}
}

func (s *Service) report(format string, args ...any) {
	if s.progress != nil {
		s.progress(fmt.Sprintf(format, args...))
	}
}

// Match resolves the given unique tracks. Order of the result mirrors input.
func (s *Service) Match(ctx context.Context, tracks []domain.Track) ([]domain.Match, error) {
	out := make([]domain.Match, len(tracks))
	pending := make([]int, 0, len(tracks)) // indexes needing text-search fallback

	// Pass 1: batch ISRC lookups.
	withISRC := make([]int, 0, len(tracks))
	for i, t := range tracks {
		if t.ISRC != "" {
			withISRC = append(withISRC, i)
		} else {
			pending = append(pending, i)
		}
	}

	for start := 0; start < len(withISRC); start += isrcBatch {
		end := min(start+isrcBatch, len(withISRC))
		batch := withISRC[start:end]
		isrcs := make([]string, len(batch))
		for j, idx := range batch {
			isrcs[j] = tracks[idx].ISRC
		}
		byISRC, err := s.catalog.SongsByISRC(ctx, isrcs)
		if err != nil {
			return nil, fmt.Errorf("isrc lookup: %w", err)
		}
		for _, idx := range batch {
			t := tracks[idx]
			cands := byISRC[strings.ToUpper(t.ISRC)]
			if best, ok := pickBest(t, cands); ok {
				out[idx] = domain.Match{Track: t, AppleID: best.ID, Confidence: domain.ConfExact, Method: domain.MethodISRC}
			} else {
				pending = append(pending, idx) // ISRC absent in Apple's catalog
			}
		}
		s.report("isrc matched %d/%d", end, len(withISRC))
	}

	// Pass 2: text-search fallback for everything still unmatched. A failed
	// search is recorded as unmatched rather than aborting the whole run, so a
	// transient rate limit can never discard the (expensive) ISRC results.
	searchErrs := 0
	for n, idx := range pending {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := sleepCtx(ctx, searchThrottle); err != nil {
			return nil, err
		}
		t := tracks[idx]
		m, err := s.searchMatch(ctx, t)
		if err != nil {
			searchErrs++
			m = domain.Match{Track: t, Confidence: domain.ConfNone, Method: domain.MethodNone, Note: "search error: " + err.Error()}
		}
		out[idx] = m
		if (n+1)%50 == 0 {
			s.report("search fallback %d/%d", n+1, len(pending))
		}
	}
	if searchErrs > 0 {
		s.report("warning: %d search lookups failed (left unmatched; re-run `match` to retry)", searchErrs)
	}

	return out, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

func (s *Service) searchMatch(ctx context.Context, t domain.Track) (domain.Match, error) {
	// Search on the base title (without "- 2016 Remaster", "(Live)", etc.) so a
	// version-suffixed Spotify title can still find the recording on Apple.
	term := cleanTitle(t.Title)
	if len(t.Artists) > 0 {
		term += " " + t.Artists[0]
	}
	cands, err := s.catalog.SearchSongs(ctx, term, 25)
	if err != nil {
		return domain.Match{}, fmt.Errorf("search %q: %w", term, err)
	}
	best, conf := pickScored(t, cands)
	if conf == domain.ConfNone {
		return domain.Match{Track: t, Confidence: domain.ConfNone, Method: domain.MethodNone, Note: "no catalog match"}, nil
	}
	return domain.Match{Track: t, AppleID: best.ID, Confidence: conf, Method: domain.MethodSearch}, nil
}

// pickBest chooses the ISRC candidate closest in duration to the source track.
func pickBest(t domain.Track, cands []domain.CatalogSong) (domain.CatalogSong, bool) {
	if len(cands) == 0 {
		return domain.CatalogSong{}, false
	}
	best := cands[0]
	bestDelta := durationDelta(t, best)
	for _, c := range cands[1:] {
		if d := durationDelta(t, c); d < bestDelta {
			best, bestDelta = c, d
		}
	}
	return best, true
}

// pickScored chooses the best search candidate and assigns a confidence.
func pickScored(t domain.Track, cands []domain.CatalogSong) (domain.CatalogSong, domain.Confidence) {
	var best domain.CatalogSong
	bestScore := -1.0
	for _, c := range cands {
		if sc := score(t, c); sc > bestScore {
			best, bestScore = c, sc
		}
	}
	switch {
	case bestScore >= 0.85:
		return best, domain.ConfHigh
	case bestScore >= 0.55:
		return best, domain.ConfLow
	default:
		return domain.CatalogSong{}, domain.ConfNone
	}
}

// score rates a candidate in [0,1] on title, artist, and duration closeness.
func score(t domain.Track, c domain.CatalogSong) float64 {
	title := titleScore(t.Title, c.Title)
	artist := 0.0
	if len(t.Artists) > 0 {
		artist = containsNorm(c.Artist, t.Artists[0])
	}
	dur := 0.0
	if durationDelta(t, c) <= durationToleranceMS {
		dur = 1.0
	}
	return 0.5*title + 0.35*artist + 0.15*dur
}

func durationDelta(t domain.Track, c domain.CatalogSong) int {
	d := t.DurationMS - c.DurationMS
	if d < 0 {
		d = -d
	}
	return d
}

// Key is a stable identity for a track: its ISRC when present, else a
// normalized title+artist. Used to dedupe tracks and to map matches back.
func Key(t domain.Track) string {
	if t.ISRC != "" {
		return "isrc:" + strings.ToUpper(t.ISRC)
	}
	return "tt:" + Normalize(t.Title) + "|" + Normalize(strings.Join(t.Artists, " "))
}

// Normalize lower-cases and strips punctuation/extra spaces for comparison.
func Normalize(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// cleanTitle drops version qualifiers Spotify appends: a " - …" suffix
// (remaster, live, single version, …) and a trailing "(…)" parenthetical.
func cleanTitle(title string) string {
	if i := strings.Index(title, " - "); i > 0 {
		title = title[:i]
	}
	if i := strings.LastIndex(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		title = title[:i]
	}
	return strings.TrimSpace(title)
}

// titleScore tolerates version suffixes: exact match scores 1.0, a match after
// stripping the source's qualifier scores 0.95, and a prefix relationship 0.8.
func titleScore(source, candidate string) float64 {
	ns, nc := Normalize(source), Normalize(candidate)
	switch {
	case ns == nc:
		return 1.0
	case Normalize(cleanTitle(source)) == nc:
		return 0.95
	case nc != "" && (strings.HasPrefix(ns, nc) || strings.HasPrefix(nc, ns)):
		return 0.8
	default:
		return 0.0
	}
}

func containsNorm(haystack, needle string) float64 {
	h, n := Normalize(haystack), Normalize(needle)
	if n != "" && strings.Contains(h, n) {
		return 1.0
	}
	return 0.0
}
