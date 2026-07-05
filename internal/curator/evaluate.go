package curator

import (
	"math"
	"sort"

	"github.com/helmedeiros/tapeit/internal/matching"
)

// EvalOptions tune the leave-one-out evaluation.
type EvalOptions struct {
	MinArtists int     // only test playlists with at least this many distinct artists (artist eval)
	MinTracks  int     // only test playlists with at least this many distinct tracks (track eval)
	Holdout    float64 // fraction of a playlist's later items to hide and try to recover
	K          int     // report Recall@K
}

// EvalResult is the outcome of a leave-one-out APC evaluation, at both the
// artist level (does curate reach the right artists?) and the track level (does
// it reach the right songs? — the thing artist recall can't see).
type EvalResult struct {
	K       int     // the K in Recall@K
	Holdout float64 // fraction held out

	Playlists      int     // playlists evaluated for artist recall
	Recall         float64 // focus-weighted affinity (what curate uses), mean Recall@K
	RPrecision     float64 // focus, mean R-precision
	BaselineRecall float64 // popularity baseline, mean Recall@K

	TrackPlaylists      int     // playlists evaluated for track recall
	TrackRecall         float64 // curate's actual output, mean track Recall@K
	TrackRPrecision     float64 // curate output, mean track R-precision
	TrackBaselineRecall float64 // popular-tracks baseline, mean track Recall@K
}

func (o EvalOptions) withDefaults() EvalOptions {
	if o.MinArtists <= 0 {
		o.MinArtists = 8
	}
	if o.MinTracks <= 0 {
		o.MinTracks = 12
	}
	if o.Holdout <= 0 {
		o.Holdout = 0.4
	}
	if o.K <= 0 {
		o.K = 20
	}
	return o
}

// Evaluate runs a leave-one-out Automatic Playlist Continuation test over the
// library: for each sufficiently large playlist, hide its later artists, seed
// from the earlier ones against a model built from every OTHER playlist, and
// measure how many held-out artists the focus-weighted ranking recovers —
// alongside a popularity baseline. This is the honest confidence signal for
// curate's candidate generation on this specific library.
func Evaluate(playlists []Playlist, opts EvalOptions) EvalResult {
	opts = opts.withDefaults()
	res := EvalResult{K: opts.K, Holdout: opts.Holdout}
	var recSum, rprecSum, baseSum float64
	var tRecSum, tRPrecSum, tBaseSum float64

	for idx, test := range playlists {
		if len(test.Tracks) > cooccurrenceMaxTracks {
			continue
		}
		rm := Build(without(playlists, idx))
		if r, ok := rm.artistEval(test, opts); ok {
			res.Playlists++
			recSum += r.recall
			rprecSum += r.rprec
			baseSum += r.base
		}
		if r, ok := rm.trackEval(test, opts); ok {
			res.TrackPlaylists++
			tRecSum += r.recall
			tRPrecSum += r.rprec
			tBaseSum += r.base
		}
	}

	if res.Playlists > 0 {
		n := float64(res.Playlists)
		res.Recall, res.RPrecision, res.BaselineRecall = recSum/n, rprecSum/n, baseSum/n
	}
	if res.TrackPlaylists > 0 {
		n := float64(res.TrackPlaylists)
		res.TrackRecall, res.TrackRPrecision, res.TrackBaselineRecall = tRecSum/n, tRPrecSum/n, tBaseSum/n
	}
	return res
}

// evalScores holds one test playlist's recall, R-precision, and baseline recall.
type evalScores struct{ recall, rprec, base float64 }

// artistEval hides a playlist's later artists and measures how well the
// focus-weighted neighbour ranking recovers them, against a popularity baseline.
func (m *Model) artistEval(test Playlist, opts EvalOptions) (evalScores, bool) {
	arts := orderedArtists(test)
	if len(arts) < opts.MinArtists {
		return evalScores{}, false
	}
	cut := int(math.Round(float64(len(arts)) * (1 - opts.Holdout)))
	seed, held := arts[:cut], arts[cut:]

	heldRec := map[string]bool{}
	for _, a := range held {
		if _, ok := m.tracks[a]; ok {
			heldRec[a] = true
		}
	}
	if len(heldRec) == 0 {
		return evalScores{}, false
	}
	seedSet := map[string]bool{}
	var known []string
	for _, s := range seed {
		if _, ok := m.tracks[s]; ok && !seedSet[s] {
			known = append(known, s)
			seedSet[s] = true
		}
	}
	ranked := m.neighbours(known, seedSet, 1)
	return evalScores{
		recall: hitRate(ranked, opts.K, heldRec),
		rprec:  hitRate(ranked, len(heldRec), heldRec),
		base:   hitRate(m.popularity(seedSet), opts.K, heldRec),
	}, true
}

// trackEval hides a playlist's later tracks and measures how many of them
// curate's actual output recovers — this is what catches good-artists-wrong-songs,
// which artistEval is blind to. Baseline is "just add the most popular songs".
func (m *Model) trackEval(test Playlist, opts EvalOptions) (evalScores, bool) {
	tracks := orderedTracks(test)
	if len(tracks) < opts.MinTracks {
		return evalScores{}, false
	}
	cut := int(math.Round(float64(len(tracks)) * (1 - opts.Holdout)))
	seed, held := tracks[:cut], tracks[cut:]

	heldRec := map[string]bool{}
	for _, t := range held {
		if m.hasTrack(t) {
			heldRec[trackKey(t)] = true
		}
	}
	if len(heldRec) == 0 {
		return evalScores{}, false
	}
	seedArtists := distinctArtistNames(seed)
	return evalScores{
		recall: trackHitRate(m.Curate(seedArtists, Options{Size: opts.K}), heldRec),
		rprec:  trackHitRate(m.Curate(seedArtists, Options{Size: len(heldRec)}), heldRec),
		base:   trackHitRate(m.popularTracks(opts.K), heldRec),
	}, true
}

// popularity ranks artists by how many playlists they appear in (descending,
// ties by name), excluding the seeds — the naive "just add popular artists"
// baseline that co-occurrence must beat to be worth anything.
func (m *Model) popularity(seedSet map[string]bool) []string {
	var arts []string
	for a := range m.freq {
		if !seedSet[a] {
			arts = append(arts, a)
		}
	}
	sort.Slice(arts, func(i, j int) bool {
		if m.freq[arts[i]] != m.freq[arts[j]] {
			return m.freq[arts[i]] > m.freq[arts[j]]
		}
		return arts[i] < arts[j]
	})
	return arts
}

// popularTracks is the track-level baseline: the top songs of the most-saved
// artists, in favorite order, until k tracks — "just add popular songs".
func (m *Model) popularTracks(k int) []Track {
	arts := make([]string, 0, len(m.freq))
	for a := range m.freq {
		arts = append(arts, a)
	}
	sort.Slice(arts, func(i, j int) bool {
		if m.freq[arts[i]] != m.freq[arts[j]] {
			return m.freq[arts[i]] > m.freq[arts[j]]
		}
		return arts[i] < arts[j]
	})
	var out []Track
	for _, a := range arts {
		for _, t := range m.tracks[a] {
			out = append(out, t)
			if len(out) >= k {
				return out
			}
		}
	}
	return out
}

// hasTrack reports whether the model's pools hold this song (by artist + base
// title), i.e. some other playlist carries it and it's recoverable.
func (m *Model) hasTrack(t Track) bool {
	bk := matching.Normalize(baseTitle(t.Title))
	for _, r := range m.tracks[matching.Normalize(t.Artist)] {
		if matching.Normalize(baseTitle(r.Title)) == bk {
			return true
		}
	}
	return false
}

func trackKey(t Track) string {
	return matching.Normalize(t.Artist) + "|" + matching.Normalize(baseTitle(t.Title))
}

func trackHitRate(cands []Track, held map[string]bool) float64 {
	if len(held) == 0 {
		return 0
	}
	hits := 0
	for _, t := range cands {
		if held[trackKey(t)] {
			hits++
		}
	}
	return float64(hits) / float64(len(held))
}

func orderedTracks(pl Playlist) []Track {
	var out []Track
	seen := map[string]bool{}
	for _, t := range pl.Tracks {
		if matching.Normalize(t.Artist) == "" {
			continue
		}
		k := trackKey(t)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}

func distinctArtistNames(tracks []Track) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range tracks {
		na := matching.Normalize(t.Artist)
		if na == "" || seen[na] {
			continue
		}
		seen[na] = true
		out = append(out, t.Artist)
	}
	return out
}

func orderedArtists(pl Playlist) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range pl.Tracks {
		na := matching.Normalize(t.Artist)
		if na == "" || seen[na] {
			continue
		}
		seen[na] = true
		out = append(out, na)
	}
	return out
}

func without(playlists []Playlist, idx int) []Playlist {
	out := make([]Playlist, 0, len(playlists)-1)
	for j, p := range playlists {
		if j != idx {
			out = append(out, p)
		}
	}
	return out
}

// hitRate is |held ∩ top-k(ranked)| / |held|.
func hitRate(ranked []string, k int, held map[string]bool) float64 {
	if len(held) == 0 {
		return 0
	}
	if k > len(ranked) {
		k = len(ranked)
	}
	hits := 0
	for _, a := range ranked[:k] {
		if held[a] {
			hits++
		}
	}
	return float64(hits) / float64(len(held))
}
