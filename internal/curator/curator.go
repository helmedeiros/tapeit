// Package curator builds a playlist from a user's own library by walking artist
// co-occurrence (which artists they group together across playlists) out from a
// seed, then sequencing the result so no two adjacent tracks share an artist.
//
// It uses only what the library already contains — no external catalog — so a
// curated playlist is a fresh recombination of songs the user already saved.
package curator

import (
	"sort"

	"github.com/helmedeiros/tapeit/internal/matching"
)

// Track is a library track the curator can place. Carry-through fields
// (Album/ISRC/DurationMS/BPM) are preserved onto the output unchanged.
type Track struct {
	Title      string
	Artist     string
	Album      string
	ISRC       string
	DurationMS int
	BPM        float64
}

// Playlist is one of the user's saved playlists, the raw co-occurrence signal.
type Playlist struct {
	Name   string
	Tracks []Track
}

// cooccurrenceMaxTracks skips very large "dump" playlists (e.g. Liked Songs)
// when counting artist affinity, so intentional grouping isn't drowned out.
const cooccurrenceMaxTracks = 250

// Options tune a curation run.
type Options struct {
	Size      int             // target number of tracks
	MinWeight int             // a neighbour must co-occur with the seed in at least this many playlists
	Breadth   int             // use at most this many (strongest-affinity) neighbours
	Exclude   map[string]bool // Key()s to skip (e.g. tracks already in a source playlist)
}

// Key is a track's identity for exclusion — normalized title + artist.
func Key(t Track) string {
	return matching.Normalize(t.Title) + "|" + matching.Normalize(t.Artist)
}

func (o Options) withDefaults() Options {
	if o.Size <= 0 {
		o.Size = 30
	}
	if o.MinWeight < 1 {
		o.MinWeight = 1
	}
	if o.Breadth <= 0 {
		o.Breadth = 12
	}
	return o
}

// Model holds artist affinity and each artist's track pool, derived from a library.
type Model struct {
	pairs   map[string]map[string]int     // norm(artist) -> norm(neighbor) -> # shared playlists
	wpairs  map[string]map[string]float64 // same, weighted by playlist focus (see countPairs)
	tracks  map[string][]Track            // norm(artist) -> unique tracks (by norm title)
	freq    map[string]int                // norm(artist) -> # playlists the artist appears in
	display map[string]string             // norm(artist) -> a display name
}

// Build derives the affinity model from a library of playlists.
func Build(playlists []Playlist) *Model {
	m := &Model{
		pairs:   map[string]map[string]int{},
		wpairs:  map[string]map[string]float64{},
		tracks:  map[string][]Track{},
		freq:    map[string]int{},
		display: map[string]string{},
	}
	for _, pl := range playlists {
		seenTrack := map[string]bool{}
		artists := map[string]bool{}
		for _, t := range pl.Tracks {
			na := matching.Normalize(t.Artist)
			if na == "" {
				continue
			}
			m.display[na] = t.Artist
			artists[na] = true
			tk := na + "|" + matching.Normalize(t.Title)
			if !seenTrack[tk] {
				seenTrack[tk] = true
				if !ownsTrack(m.tracks[na], t) {
					m.tracks[na] = append(m.tracks[na], t)
				}
			}
		}
		if len(pl.Tracks) <= cooccurrenceMaxTracks {
			m.countPairs(artists)
			for a := range artists {
				m.freq[a]++
			}
		}
	}
	return m
}

// Knows reports whether the artist appears anywhere in the library.
func (m *Model) Knows(artist string) bool {
	_, ok := m.tracks[matching.Normalize(artist)]
	return ok
}

// Separate reorders tracks so no two adjacent share an artist. Exported so a
// caller can re-sequence a set it has combined (e.g. library + discovery).
func Separate(tracks []Track) []Track { return separate(tracks) }

func ownsTrack(pool []Track, t Track) bool {
	title := matching.Normalize(t.Title)
	for _, p := range pool {
		if matching.Normalize(p.Title) == title {
			return true
		}
	}
	return false
}

// countPairs records artist co-occurrence for one playlist. It tracks both a raw
// shared-playlist count and a weighted score: each playlist distributes a fixed
// unit of affinity, so in a k-artist playlist every pair contributes 1/(k-1).
// A focused 12-artist set therefore counts for far more per pair than a
// 150-artist grab-bag, surfacing genuine affinity over incidental co-occurrence.
func (m *Model) countPairs(artists map[string]bool) {
	list := make([]string, 0, len(artists))
	for a := range artists {
		list = append(list, a)
	}
	if len(list) < 2 {
		return
	}
	share := 1.0 / float64(len(list)-1)
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			m.addPair(list[i], list[j], share)
			m.addPair(list[j], list[i], share)
		}
	}
}

func (m *Model) addPair(a, b string, share float64) {
	if m.pairs[a] == nil {
		m.pairs[a] = map[string]int{}
		m.wpairs[a] = map[string]float64{}
	}
	m.pairs[a][b]++
	m.wpairs[a][b] += share
}

// Curate returns up to opts.Size tracks around the seed artists: the seeds' own
// tracks plus those of their strongest-affinity neighbours, sequenced so no two
// adjacent tracks share an artist. Empty if none of the seeds are in the library.
//
// Seeding from several artists is materially stronger than one: a single seed
// barely constrains what belongs (see lab/experiments/RESULTS.md), so affinity
// is summed across all seeds. Neighbours are ranked by focus-weighted affinity
// and capped at opts.Breadth to stay focused on the strongest associations.
func (m *Model) Curate(seeds []string, opts Options) []Track {
	var known []string
	seen := map[string]bool{}
	for _, s := range seeds {
		ns := matching.Normalize(s)
		if _, ok := m.tracks[ns]; ok && !seen[ns] {
			known = append(known, ns)
			seen[ns] = true
		}
	}
	if len(known) == 0 {
		return nil
	}
	opts = opts.withDefaults()
	return separate(m.gather(known, seen, opts))
}

// gather collects tracks round-robin across the seeds and their top neighbours,
// one per artist per pass, until it reaches opts.Size.
func (m *Model) gather(seeds []string, seedSet map[string]bool, opts Options) []Track {
	nbs := m.neighbours(seeds, seedSet, opts.MinWeight)
	if len(nbs) > opts.Breadth {
		nbs = nbs[:opts.Breadth]
	}
	order := append(append([]string{}, seeds...), nbs...)
	pos := map[string]int{}
	var out []Track
	for progressed := true; len(out) < opts.Size && progressed; {
		progressed = false
		for _, a := range order {
			if len(out) >= opts.Size {
				break
			}
			pool := m.artistTracksSorted(a)
			for pos[a] < len(pool) && opts.Exclude[Key(pool[pos[a]])] {
				pos[a]++
			}
			if pos[a] < len(pool) {
				out = append(out, pool[pos[a]])
				pos[a]++
				progressed = true
			}
		}
	}
	return out
}

// neighbours returns artists co-occurring with any seed (in at least minWeight
// playlists with that seed), ranked by affinity summed across the seeds and
// focus-weighted (ties broken by name), excluding the seeds themselves.
func (m *Model) neighbours(seeds []string, seedSet map[string]bool, minWeight int) []string {
	score := map[string]float64{}
	maxCount := map[string]int{}
	for _, s := range seeds {
		for a, count := range m.pairs[s] {
			if seedSet[a] {
				continue
			}
			if count > maxCount[a] {
				maxCount[a] = count
			}
			score[a] += m.wpairs[s][a]
		}
	}
	type nb struct {
		artist string
		score  float64
	}
	var nbs []nb
	for a := range score {
		if maxCount[a] >= minWeight {
			nbs = append(nbs, nb{a, score[a]})
		}
	}
	sort.Slice(nbs, func(i, j int) bool {
		if nbs[i].score != nbs[j].score {
			return nbs[i].score > nbs[j].score
		}
		return nbs[i].artist < nbs[j].artist
	})
	out := make([]string, len(nbs))
	for i, n := range nbs {
		out[i] = n.artist
	}
	return out
}

func (m *Model) artistTracksSorted(artist string) []Track {
	pool := append([]Track(nil), m.tracks[artist]...)
	sort.Slice(pool, func(i, j int) bool { return pool[i].Title < pool[j].Title })
	return pool
}

// separate greedily reorders so no two adjacent tracks share an artist,
// preferring the artist with the most remaining tracks (spreads them out).
func separate(tracks []Track) []Track {
	remaining := map[string]int{}
	for _, t := range tracks {
		remaining[matching.Normalize(t.Artist)]++
	}
	pool := append([]Track(nil), tracks...)
	var out []Track
	last := ""
	for len(pool) > 0 {
		idx := -1
		for i, t := range pool {
			na := matching.Normalize(t.Artist)
			if na == last {
				continue
			}
			if idx == -1 || remaining[na] > remaining[matching.Normalize(pool[idx].Artist)] {
				idx = i
			}
		}
		if idx == -1 { // only the last artist remains; accept the repeat
			idx = 0
		}
		pick := pool[idx]
		out = append(out, pick)
		remaining[matching.Normalize(pick.Artist)]--
		last = matching.Normalize(pick.Artist)
		pool = append(pool[:idx], pool[idx+1:]...)
	}
	return out
}
