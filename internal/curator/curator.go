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
	Size      int // target number of tracks
	MinWeight int // a neighbour must co-occur with the seed in at least this many playlists
	Breadth   int // use at most this many (strongest-PMI) neighbours
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
	pairs   map[string]map[string]int // norm(artist) -> norm(neighbor) -> co-occurrence count
	tracks  map[string][]Track        // norm(artist) -> unique tracks (by norm title)
	display map[string]string         // norm(artist) -> a display name
}

// Build derives the affinity model from a library of playlists.
func Build(playlists []Playlist) *Model {
	m := &Model{
		pairs:   map[string]map[string]int{},
		tracks:  map[string][]Track{},
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
		}
	}
	return m
}

func ownsTrack(pool []Track, t Track) bool {
	title := matching.Normalize(t.Title)
	for _, p := range pool {
		if matching.Normalize(p.Title) == title {
			return true
		}
	}
	return false
}

func (m *Model) countPairs(artists map[string]bool) {
	list := make([]string, 0, len(artists))
	for a := range artists {
		list = append(list, a)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			a, b := list[i], list[j]
			if m.pairs[a] == nil {
				m.pairs[a] = map[string]int{}
			}
			if m.pairs[b] == nil {
				m.pairs[b] = map[string]int{}
			}
			m.pairs[a][b]++
			m.pairs[b][a]++
		}
	}
}

// Curate returns up to opts.Size tracks around the seed artist: the seed's own
// tracks plus those of its strongest-affinity neighbours, sequenced so no two
// adjacent tracks share an artist. Empty if the seed isn't in the library.
//
// Neighbours are ranked by how many playlists they share with the seed and
// capped at opts.Breadth, so the result stays focused on the seed's strongest
// affinities instead of padding with one-off co-occurrences.
func (m *Model) Curate(seed string, opts Options) []Track {
	ns := matching.Normalize(seed)
	if _, ok := m.tracks[ns]; !ok {
		return nil
	}
	opts = opts.withDefaults()
	selected := m.gather(ns, opts)
	return separate(selected)
}

// gather collects tracks round-robin across the seed and its top neighbours,
// one per artist per pass, until it reaches opts.Size.
func (m *Model) gather(seed string, opts Options) []Track {
	nbs := m.neighbours(seed, opts.MinWeight)
	if len(nbs) > opts.Breadth {
		nbs = nbs[:opts.Breadth]
	}
	order := append([]string{seed}, nbs...)
	pos := map[string]int{}
	var out []Track
	for progressed := true; len(out) < opts.Size && progressed; {
		progressed = false
		for _, a := range order {
			if len(out) >= opts.Size {
				break
			}
			pool := m.artistTracksSorted(a)
			if pos[a] < len(pool) {
				out = append(out, pool[pos[a]])
				pos[a]++
				progressed = true
			}
		}
	}
	return out
}

// neighbours returns the seed's co-occurring artists (sharing at least minWeight
// playlists), most-shared first (ties broken by name for determinism).
func (m *Model) neighbours(seed string, minWeight int) []string {
	type nb struct {
		artist string
		weight int
	}
	var nbs []nb
	for a, w := range m.pairs[seed] {
		if w >= minWeight {
			nbs = append(nbs, nb{a, w})
		}
	}
	sort.Slice(nbs, func(i, j int) bool {
		if nbs[i].weight != nbs[j].weight {
			return nbs[i].weight > nbs[j].weight
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
