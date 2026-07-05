package curator

import (
	"math"
	"sort"

	"github.com/helmedeiros/tapeit/internal/matching"
)

// EvalOptions tune the leave-one-out evaluation.
type EvalOptions struct {
	MinArtists int     // only test playlists with at least this many distinct artists
	Holdout    float64 // fraction of a playlist's later artists to hide and try to recover
	K          int     // report Recall@K
}

// EvalResult is the outcome of a leave-one-out APC evaluation.
type EvalResult struct {
	Playlists      int     // test playlists evaluated
	K              int     // the K in Recall@K
	Holdout        float64 // fraction held out
	Recall         float64 // focus-weighted affinity (what curate uses), mean Recall@K
	RPrecision     float64 // focus, mean R-precision
	BaselineRecall float64 // popularity baseline, mean Recall@K
}

func (o EvalOptions) withDefaults() EvalOptions {
	if o.MinArtists <= 0 {
		o.MinArtists = 8
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

	for idx, test := range playlists {
		if len(test.Tracks) > cooccurrenceMaxTracks {
			continue
		}
		arts := orderedArtists(test)
		if len(arts) < opts.MinArtists {
			continue
		}
		cut := int(math.Round(float64(len(arts)) * (1 - opts.Holdout)))
		seed, held := arts[:cut], arts[cut:]

		rm := Build(without(playlists, idx))

		heldRec := map[string]bool{}
		for _, a := range held {
			if _, ok := rm.tracks[a]; ok {
				heldRec[a] = true
			}
		}
		if len(heldRec) == 0 {
			continue
		}

		seedSet := map[string]bool{}
		var known []string
		for _, s := range seed {
			if _, ok := rm.tracks[s]; ok && !seedSet[s] {
				known = append(known, s)
				seedSet[s] = true
			}
		}
		ranked := rm.neighbours(known, seedSet, 1)

		res.Playlists++
		recSum += hitRate(ranked, opts.K, heldRec)
		rprecSum += hitRate(ranked, len(heldRec), heldRec)
		baseSum += hitRate(rm.popularity(seedSet), opts.K, heldRec)
	}

	if res.Playlists > 0 {
		n := float64(res.Playlists)
		res.Recall = recSum / n
		res.RPrecision = rprecSum / n
		res.BaselineRecall = baseSum / n
	}
	return res
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
