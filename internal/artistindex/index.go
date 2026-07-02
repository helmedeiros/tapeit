// Package artistindex is a local, persistent cache of online artist
// relationships (similar artists and their top tracks). It lets the curator
// discover artists beyond the user's own library without hitting the network
// on every run — fetched once, kept locally, reused thereafter.
package artistindex

import (
	"context"
	"encoding/json"
	"os"

	"github.com/helmedeiros/tapeit/internal/deezer"
	"github.com/helmedeiros/tapeit/internal/matching"
)

// Source is the online provider the index fetches from (satisfied by *deezer.Client).
type Source interface {
	ArtistID(ctx context.Context, name string) (int64, bool, error)
	RelatedArtists(ctx context.Context, id int64) ([]string, error)
	TopTracks(ctx context.Context, id int64, limit int) ([]deezer.Track, error)
}

// Track is a discovered track (title + its artist).
type Track struct {
	Title  string `json:"title"`
	Artist string `json:"artist"`
}

type entry struct {
	ID      int64    `json:"id"`
	Related []string `json:"related,omitempty"`
	Top     []Track  `json:"top,omitempty"`
}

// Index is the on-disk artist-relationship cache.
type Index struct {
	path    string
	entries map[string]entry // key: normalized artist name
	dirty   bool
}

// Load reads the index from path, returning an empty index if it doesn't exist.
func Load(path string) (*Index, error) {
	ix := &Index{path: path, entries: map[string]entry{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ix, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &ix.entries); err != nil {
		return nil, err
	}
	return ix, nil
}

// Save writes the index back to disk if it changed.
func (ix *Index) Save() error {
	if !ix.dirty {
		return nil
	}
	data, err := json.MarshalIndent(ix.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ix.path, append(data, '\n'), 0o644)
}

func (ix *Index) resolve(ctx context.Context, src Source, name string) (entry, string, error) {
	key := matching.Normalize(name)
	e, ok := ix.entries[key]
	if ok && e.ID != 0 {
		return e, key, nil
	}
	id, found, err := src.ArtistID(ctx, name)
	if err != nil {
		return entry{}, key, err
	}
	if !found {
		return entry{}, key, nil
	}
	e.ID = id
	ix.entries[key] = e
	ix.dirty = true
	return e, key, nil
}

// Related returns artists similar to name, fetching and caching on a miss.
func (ix *Index) Related(ctx context.Context, src Source, name string) ([]string, error) {
	e, key, err := ix.resolve(ctx, src, name)
	if err != nil || e.ID == 0 {
		return nil, err
	}
	if e.Related != nil {
		return e.Related, nil
	}
	related, err := src.RelatedArtists(ctx, e.ID)
	if err != nil {
		return nil, err
	}
	e.Related = related
	ix.entries[key] = e
	ix.dirty = true
	return related, nil
}

// TopTracks returns up to n of an artist's top tracks, fetching and caching on a miss.
func (ix *Index) TopTracks(ctx context.Context, src Source, name string, n int) ([]Track, error) {
	e, key, err := ix.resolve(ctx, src, name)
	if err != nil || e.ID == 0 {
		return nil, err
	}
	if len(e.Top) < n {
		tracks, err := src.TopTracks(ctx, e.ID, n)
		if err != nil {
			return nil, err
		}
		e.Top = e.Top[:0]
		for _, t := range tracks {
			e.Top = append(e.Top, Track{Title: t.Title, Artist: name})
		}
		ix.entries[key] = e
		ix.dirty = true
	}
	if len(e.Top) > n {
		return e.Top[:n], nil
	}
	return e.Top, nil
}
