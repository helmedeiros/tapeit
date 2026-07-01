package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/curator"
)

func cmdCurate(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("curate", flag.ContinueOnError)
	seed := fs.String("seed", "", "seed artist to build the playlist around (required)")
	size := fs.Int("size", 30, "target number of tracks")
	breadth := fs.Int("breadth", 12, "how many neighbouring artists to draw from (lower = tighter)")
	minWeight := fs.Int("min-affinity", 1, "min playlists a neighbour must share with the seed")
	name := fs.String("name", "", "playlist name (default: \"Around <seed>\")")
	dir := fs.String("dir", "playlists", "library directory to draw from")
	out := fs.String("out", "playlists", "directory to write the new playlist into")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*seed) == "" {
		return fmt.Errorf("missing --seed (an artist in your library)")
	}

	lib, err := loadLibrary(*dir)
	if err != nil {
		return err
	}
	model := curator.Build(lib)
	tracks := model.Curate(*seed, curator.Options{Size: *size, Breadth: *breadth, MinWeight: *minWeight})
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks found around %q — is that artist in your library (under %s/)?", *seed, *dir)
	}

	plName := *name
	if plName == "" {
		plName = "Around " + *seed
	}
	doc := playlistDoc{Name: plName, Tracks: fromCuratorTracks(tracks)}
	path := filepath.Join(*out, slugify(plName)+".json")
	if err := writeJSON(path, doc); err != nil {
		return err
	}

	fmt.Printf("✓ curated %q — %d tracks from %d artists → %s\n",
		plName, len(tracks), distinctArtists(tracks), path)
	fmt.Println("  build it on Apple Music with:  tapeit create --from " + path)
	return nil
}

func loadLibrary(dir string) ([]curator.Playlist, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no playlist files in %s", dir)
	}
	var lib []curator.Playlist
	for _, f := range files {
		doc, err := loadDoc(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		lib = append(lib, curator.Playlist{Name: doc.Name, Tracks: toCuratorTracks(doc.Tracks)})
	}
	return lib, nil
}

func toCuratorTracks(tracks []playlistTrack) []curator.Track {
	out := make([]curator.Track, 0, len(tracks))
	for _, t := range tracks {
		ct := curator.Track{Title: t.Title, Artist: t.Artist, Album: t.Album, ISRC: t.ISRC, DurationMS: t.DurationMS}
		if t.Features != nil {
			ct.BPM = t.Features.BPM
		}
		out = append(out, ct)
	}
	return out
}

func fromCuratorTracks(tracks []curator.Track) []playlistTrack {
	out := make([]playlistTrack, 0, len(tracks))
	for _, t := range tracks {
		pt := playlistTrack{Title: t.Title, Artist: t.Artist, Album: t.Album, ISRC: t.ISRC, DurationMS: t.DurationMS}
		if t.BPM > 0 {
			pt.Features = &trackFeatures{BPM: t.BPM, Source: "deezer"}
		}
		out = append(out, pt)
	}
	return out
}

func distinctArtists(tracks []curator.Track) int {
	seen := map[string]bool{}
	for _, t := range tracks {
		seen[t.Artist] = true
	}
	return len(seen)
}
