package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/artistindex"
	"github.com/helmedeiros/tapeit/internal/config"
	"github.com/helmedeiros/tapeit/internal/curator"
	"github.com/helmedeiros/tapeit/internal/deezer"
)

func cmdCurate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("curate", flag.ContinueOnError)
	seed := fs.String("seed", "", "seed artist(s) to build around, comma-separated (required)")
	size := fs.Int("size", 30, "target number of tracks from your library")
	breadth := fs.Int("breadth", 12, "how many neighbouring artists to draw from (lower = tighter)")
	minWeight := fs.Int("min-affinity", 1, "min playlists a neighbour must share with the seed")
	discover := fs.Int("discover", 0, "also add up to N tracks by similar artists you don't own yet (online)")
	name := fs.String("name", "", "playlist name (default: \"Around <seed>\")")
	dir := fs.String("dir", "playlists", "library directory to draw from")
	out := fs.String("out", "playlists", "directory to write the new playlist into")
	force := fs.Bool("force", false, "overwrite the output file if it already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	seeds := splitSeeds(*seed)
	if len(seeds) == 0 {
		return fmt.Errorf("missing --seed (one or more artists in your library, comma-separated)")
	}

	lib, err := loadLibrary(*dir)
	if err != nil {
		return err
	}
	model := curator.Build(lib)
	tracks := model.Curate(seeds, curator.Options{Size: *size, Breadth: *breadth, MinWeight: *minWeight})
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks found around %s — are those artists in your library (under %s/)?", strings.Join(seeds, ", "), *dir)
	}

	discovered := 0
	if *discover > 0 {
		tracks, discovered, err = addDiscovery(ctx, model, tracks, seeds, *discover)
		if err != nil {
			return err
		}
		tracks = curator.Separate(tracks)
	}

	plName := *name
	if plName == "" {
		plName = "Around " + strings.Join(seeds, " & ")
	}
	path := filepath.Join(*out, slugify(plName)+".json")
	if !*force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists — pass --force to overwrite, or --name to write elsewhere", path)
		}
	}
	doc := playlistDoc{Name: plName, Tracks: fromCuratorTracks(tracks)}
	if err := writeJSON(path, doc); err != nil {
		return err
	}

	fmt.Printf("✓ curated %q — %d tracks from %d artists", plName, len(tracks), distinctArtists(tracks))
	if discovered > 0 {
		fmt.Printf(" (%d discovered from new artists)", discovered)
	}
	fmt.Printf(" → %s\n", path)
	fmt.Println("  build it on Apple Music with:  tapeit create --from " + path)
	return nil
}

// addDiscovery appends up to n tracks by artists similar to the seeds that the
// user doesn't already own, using the local (network-backed) artist index. It
// interleaves each seed's related artists so multiple seeds are all represented.
func addDiscovery(ctx context.Context, model *curator.Model, tracks []curator.Track, seeds []string, n int) ([]curator.Track, int, error) {
	path, err := config.ArtistIndexPath()
	if err != nil {
		return tracks, 0, err
	}
	ix, err := artistindex.Load(path)
	if err != nil {
		return tracks, 0, err
	}
	src := deezer.NewClient()

	related := make([][]string, len(seeds))
	for i, s := range seeds {
		if related[i], err = ix.Related(ctx, src, s); err != nil {
			return tracks, 0, err
		}
	}

	added := 0
	used := map[string]bool{}
	for depth := 0; added < n; depth++ {
		progressed := false
		for _, list := range related {
			if depth >= len(list) {
				continue
			}
			progressed = true
			artist := list[depth]
			if added >= n || used[artist] || model.Knows(artist) {
				continue
			}
			used[artist] = true
			tops, err := ix.TopTracks(ctx, src, artist, 2)
			if err != nil {
				return tracks, added, err
			}
			tracks, added = appendTops(tracks, tops, artist, added, n)
		}
		if !progressed {
			break
		}
	}
	if err := ix.Save(); err != nil {
		return tracks, added, err
	}
	return tracks, added, nil
}

// appendTops adds an artist's top tracks up to the discovery cap n.
func appendTops(tracks []curator.Track, tops []artistindex.Track, artist string, added, n int) ([]curator.Track, int) {
	for _, t := range tops {
		if added >= n {
			break
		}
		tracks = append(tracks, curator.Track{Title: t.Title, Artist: artist})
		added++
	}
	return tracks, added
}

// splitSeeds parses a comma-separated --seed value into trimmed artist names.
func splitSeeds(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
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
