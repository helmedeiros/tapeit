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

// maxConsecutiveSearchErrors aborts the search pass when this many lookups fail
// back-to-back (Apple is throttling) rather than grinding to an all-unmatched
// result.
const maxConsecutiveSearchErrors = 8

// Service turns tracks into matches using one or more catalogs.
type Service struct {
	catalogs []domain.CatalogPort
	progress func(string)
}

// New builds a matching service over a single catalog. progress may be nil.
func New(catalog domain.CatalogPort, progress func(string)) *Service {
	return NewChained(progress, catalog)
}

// NewChained builds a matching service that consults catalogs in order,
// keeping the best answer any of them gives and stopping as soon as one is
// high-confidence.
//
// This exists because no single catalog index is complete. The iTunes Search
// API is the preferred primary (it has its own rate-limit quota, separate from
// the rest of the Apple Music API) but its index is missing recordings that
// amp-api has — Gorillaz' Song Machine, for one. Worse, it does not fail
// loudly: asked for a song it lacks, it returns 25 plausible-but-wrong results,
// so an empty-result check would never trigger a fallback. Only the score can
// tell the difference, which is why chaining lives here and not behind a
// composite CatalogPort.
func NewChained(progress func(string), catalogs ...domain.CatalogPort) *Service {
	return &Service{catalogs: catalogs, progress: progress}
}

// confidenceRank orders Confidence so the better of two matches can be chosen.
func confidenceRank(c domain.Confidence) int {
	switch c {
	case domain.ConfExact:
		return 3
	case domain.ConfHigh:
		return 2
	case domain.ConfLow:
		return 1
	default:
		return 0
	}
}

func (s *Service) report(format string, args ...any) {
	if s.progress != nil {
		s.progress(fmt.Sprintf(format, args...))
	}
}

// Match resolves the given unique tracks. Order of the result mirrors input.
func (s *Service) Match(ctx context.Context, tracks []domain.Track) ([]domain.Match, error) {
	out := make([]domain.Match, len(tracks))
	withISRC, pending := classify(tracks, out)

	// Pass 1: batch ISRC lookups.
	for start := 0; start < len(withISRC); start += isrcBatch {
		end := min(start+isrcBatch, len(withISRC))
		batch := withISRC[start:end]
		isrcs := make([]string, len(batch))
		for j, idx := range batch {
			isrcs[j] = tracks[idx].ISRC
		}
		byISRC, err := s.songsByISRC(ctx, isrcs)
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
	// transient rate limit can never discard the (expensive) ISRC results. But
	// if many searches fail in a row, Apple is throttling us hard — abort fast
	// instead of grinding and producing an all-unmatched result.
	searchErrs, consecutive := 0, 0
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
			consecutive++
			if consecutive >= maxConsecutiveSearchErrors {
				return nil, fmt.Errorf("aborting: %d searches failed in a row — Apple is rate-limiting search; try again later (%w)", consecutive, err)
			}
			m = domain.Match{Track: t, Confidence: domain.ConfNone, Method: domain.MethodNone, Note: "search error: " + err.Error()}
		} else {
			consecutive = 0
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

// classify sorts track indexes by how they will be resolved, writing the ones
// that need no lookup (hand-pinned catalog ids) straight into out.
func classify(tracks []domain.Track, out []domain.Match) (withISRC, pending []int) {
	withISRC = make([]int, 0, len(tracks))
	pending = make([]int, 0, len(tracks))
	for i, t := range tracks {
		switch {
		case t.AppleID != "":
			// Pinned by hand to an exact catalog song. Trusted over every
			// lookup, and costs no request.
			out[i] = domain.Match{Track: t, AppleID: t.AppleID, Confidence: domain.ConfExact, Method: domain.MethodManual}
		case t.ISRC != "":
			withISRC = append(withISRC, i)
		default:
			pending = append(pending, i)
		}
	}
	return withISRC, pending
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// songsByISRC asks each catalog in turn for the ISRCs still unresolved, so a
// primary with no ISRC index (the iTunes Search API) costs nothing but does not
// block a later catalog that has one.
func (s *Service) songsByISRC(ctx context.Context, isrcs []string) (map[string][]domain.CatalogSong, error) {
	out := make(map[string][]domain.CatalogSong, len(isrcs))
	var firstErr error
	answered := false

	remaining := make([]string, len(isrcs))
	copy(remaining, isrcs)

	for _, c := range s.catalogs {
		if len(remaining) == 0 {
			break
		}
		got, err := c.SongsByISRC(ctx, remaining)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered = true
		still := remaining[:0:0]
		for _, code := range remaining {
			key := strings.ToUpper(code)
			if songs := got[key]; len(songs) > 0 {
				out[key] = songs
			} else {
				still = append(still, code)
			}
		}
		remaining = still
	}

	// Only fail when no catalog managed to answer at all; a single rate-limited
	// adapter must not sink a lookup another one could serve.
	if !answered && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func (s *Service) searchMatch(ctx context.Context, t domain.Track) (domain.Match, error) {
	// Search on the base title (without "- 2016 Remaster", "(Live)", etc.) so a
	// version-suffixed Spotify title can still find the recording on Apple.
	term := cleanTitle(t.Title)
	if len(t.Artists) > 0 {
		term += " " + t.Artists[0]
	}

	var (
		best     domain.CatalogSong
		bestConf = domain.ConfNone
		firstErr error
		answered bool
	)
	for _, c := range s.catalogs {
		cands, err := c.SearchSongs(ctx, term, 25)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		answered = true
		if song, conf := pickScored(t, cands); confidenceRank(conf) > confidenceRank(bestConf) {
			best, bestConf = song, conf
		}
		if bestConf == domain.ConfHigh {
			break // good enough; do not spend a request on the next catalog
		}
	}

	if !answered && firstErr != nil {
		return domain.Match{}, fmt.Errorf("search %q: %w", term, firstErr)
	}
	if bestConf == domain.ConfNone {
		return domain.Match{Track: t, Confidence: domain.ConfNone, Method: domain.MethodNone, Note: "no catalog match"}, nil
	}
	return domain.Match{Track: t, AppleID: best.ID, Confidence: bestConf, Method: domain.MethodSearch}, nil
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
