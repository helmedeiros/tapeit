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
	seed := fs.String("seed", "", "seed artist(s) to build around, comma-separated")
	seedPlaylist := fs.String("seed-playlist", "", "extend an existing playlist: seed from all its artists, exclude its tracks (slug or path)")
	size := fs.Int("size", 30, "target number of tracks from your library")
	breadth := fs.Int("breadth", 12, "how many neighbouring artists to draw from (lower = tighter)")
	minWeight := fs.Int("min-affinity", 1, "min playlists a neighbour must share with the seed")
	discover := fs.Int("discover", 0, "also add up to N tracks by similar artists you don't own yet (online)")
	evaluate := fs.Bool("evaluate", false, "leave-one-out APC test of the library's affinity signal (no playlist written)")
	flow := fs.String("flow", "none", "tempo shape once enriched: none|smooth|arc")
	name := fs.String("name", "", "playlist name (default: \"Around <seed>\")")
	dir := fs.String("dir", "playlists", "library directory to draw from")
	out := fs.String("out", "playlists", "directory to write the new playlist into")
	force := fs.Bool("force", false, "overwrite the output file if it already exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *evaluate {
		return runEvaluate(*dir)
	}
	flowMode, ok := curator.ParseFlow(*flow)
	if !ok {
		return fmt.Errorf("unknown --flow %q (use none|smooth|arc)", *flow)
	}

	explicit := splitSeeds(*seed)
	seeds, sourceName, exclude, err := assembleSeeds(explicit, *seedPlaylist, *dir)
	if err != nil {
		return err
	}
	if len(seeds) == 0 {
		return fmt.Errorf("give --seed (comma-separated artists) or --seed-playlist")
	}

	lib, err := loadLibrary(*dir)
	if err != nil {
		return err
	}
	model := curator.Build(lib)
	tracks := model.Curate(seeds, curator.Options{Size: *size, Breadth: *breadth, MinWeight: *minWeight, Exclude: exclude})
	if len(tracks) == 0 {
		return fmt.Errorf("no tracks found — are the seed artists in your library (under %s/)?", *dir)
	}

	discovered := 0
	if *discover > 0 {
		discoverSeeds := explicit
		if len(discoverSeeds) == 0 {
			discoverSeeds = seeds
		}
		tracks, discovered, err = addDiscovery(ctx, model, tracks, discoverSeeds, *discover)
		if err != nil {
			return err
		}
	}
	tracks = curator.Sequence(tracks, flowMode)

	plName := curateName(*name, sourceName, explicit)
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

// curateName picks the playlist name: an explicit --name wins, else "More Like
// <source>" when continuing a playlist, else "Around <seeds>".
func curateName(name, sourceName string, explicit []string) string {
	switch {
	case name != "":
		return name
	case sourceName != "":
		return "More Like " + sourceName
	default:
		return "Around " + strings.Join(explicit, " & ")
	}
}

func runEvaluate(dir string) error {
	lib, err := loadLibrary(dir)
	if err != nil {
		return err
	}
	r := curator.Evaluate(lib, curator.EvalOptions{})
	printEvaluation(r)
	return gateEvaluation(r)
}

func printEvaluation(r curator.EvalResult) {
	if r.Playlists == 0 && r.TrackPlaylists == 0 {
		fmt.Println("not enough playlists to evaluate — need a few with 8+ distinct artists / 12+ tracks")
		return
	}
	fmt.Printf("Leave-one-out APC eval — %.0f%% of each playlist held out\n\n", r.Holdout*100)
	fmt.Printf("  %-20s Recall@%d   R-precision\n", "level / strategy", r.K)
	fmt.Printf("  artists (%d playlists)\n", r.Playlists)
	fmt.Printf("    %-18s %7.3f %11.3f\n", "focus (curate)", r.Recall, r.RPrecision)
	fmt.Printf("    %-18s %7.3f %11s\n", "popularity", r.BaselineRecall, "—")
	fmt.Printf("  tracks (%d playlists)\n", r.TrackPlaylists)
	fmt.Printf("    %-18s %7.3f %11.3f\n", "curate", r.TrackRecall, r.TrackRPrecision)
	fmt.Printf("    %-18s %7.3f %11s\n", "popular songs", r.TrackBaselineRecall, "—")
	fmt.Printf("\ncurate beats the popularity baseline by %.2f× on artists, %.2f× on tracks.\n",
		lift(r.Recall, r.BaselineRecall), lift(r.TrackRecall, r.TrackBaselineRecall))
}

func lift(focus, base float64) float64 {
	if base <= 0 {
		return 0
	}
	return focus / base
}

// gateEvaluation fails (non-zero exit) if curate's signal no longer beats a
// popularity baseline — a CI regression guard on candidate quality.
func gateEvaluation(r curator.EvalResult) error {
	if r.Playlists > 0 && r.Recall <= r.BaselineRecall {
		return fmt.Errorf("artist recall %.3f does not beat popularity %.3f", r.Recall, r.BaselineRecall)
	}
	if r.TrackPlaylists > 0 && r.TrackRecall <= r.TrackBaselineRecall {
		return fmt.Errorf("track recall %.3f does not beat popularity %.3f", r.TrackRecall, r.TrackBaselineRecall)
	}
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

	if len(seeds) > maxDiscoverySeeds {
		seeds = seeds[:maxDiscoverySeeds]
	}
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

// assembleSeeds combines the explicit --seed artists with those of a
// --seed-playlist (if given), returning the seed list, the source playlist's
// name, and the set of its tracks to exclude from the result.
func assembleSeeds(explicit []string, seedPlaylist, dir string) (seeds []string, sourceName string, exclude map[string]bool, err error) {
	seeds = append([]string{}, explicit...)
	if seedPlaylist == "" {
		return seeds, "", nil, nil
	}
	src, err := loadDoc(resolvePlaylist(seedPlaylist, dir))
	if err != nil {
		return nil, "", nil, fmt.Errorf("read --seed-playlist: %w", err)
	}
	exclude = map[string]bool{}
	for _, t := range src.Tracks {
		seeds = append(seeds, t.Artist)
		exclude[curator.Key(curator.Track{Title: t.Title, Artist: t.Artist})] = true
	}
	return seeds, src.Name, exclude, nil
}

// resolvePlaylist turns a --seed-playlist value into a file path: a bare slug is
// looked up under dir; a path or *.json value is used as-is.
func resolvePlaylist(v, dir string) string {
	if strings.HasSuffix(v, ".json") || strings.ContainsRune(v, filepath.Separator) {
		return v
	}
	return filepath.Join(dir, slugify(v)+".json")
}

// maxDiscoverySeeds bounds how many seeds we fan out to online — a whole
// seed-playlist can carry hundreds of artists, one Deezer lookup each.
const maxDiscoverySeeds = 5

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
