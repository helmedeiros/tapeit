package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/helmedeiros/tapeit/internal/apple"
	"github.com/helmedeiros/tapeit/internal/config"
	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/vinyl"
)

// yearInName pulls the list's year out of a playlist name ("Your Top Songs 2019").
var yearInName = regexp.MustCompile(`(19|20)\d{2}`)

func cmdVinyl(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("vinyl", flag.ContinueOnError)
	match := fs.String("match", "Your Top Songs", "library playlists whose name contains this are the ranked lists")
	size := fs.Int("size", 20, "how many albums to recommend")
	perArtist := fs.Int("max-per-artist", 2, "cap on albums per artist (0 = unlimited)")
	withST := fs.Bool("include-soundtracks", false, "admit film and show soundtracks")
	minTracks := fs.Int("min-tracks", 7, "fewest tracks for a record to count as an album")
	evaluate := fs.Bool("evaluate", false, "measure the ranking against a naive top-tracks baseline")
	refresh := fs.Bool("refresh", false, "ignore the cached album metadata")
	cachedOnly := fs.Bool("cached-only", false,
		"rank from cached album metadata alone, resolving nothing (useful while the catalog is rate-limiting)")
	alpha := fs.Float64("rank-alpha", vinyl.DefaultRankAlpha,
		"steepness of the assumed play curve; 0.5 makes first place ~10x the hundredth")
	censor := fs.Float64("censoring-credit", vinyl.DefaultCensoringCredit,
		"how much of a record's unheard remainder to credit as played below the cutoff (0 disables)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	creds, err := loadAppleCreds()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth apple` first)", err)
	}
	if err := creds.Validate(); err != nil {
		return err
	}
	client := apple.NewClient(creds)

	apps, first, last, err := readRankedLists(ctx, client, *match)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("no ranked lists found matching %q in your library", *match)
	}
	fmt.Printf("read %d appearances from %d–%d\n", len(apps), first, last)

	lib0 := libraryDepth()
	meta, err := albumMetadata(ctx, client, apps, lib0, *refresh, *cachedOnly)
	if err != nil {
		return err
	}
	lib := lib0

	o := vinyl.DefaultOptions(first, last)
	o.Size, o.MaxPerArtist, o.IncludeSoundtracks, o.MinTracks = *size, *perArtist, *withST, *minTracks
	o.RankAlpha, o.CensoringCredit = *alpha, *censor

	if *evaluate {
		fmt.Println()
		fmt.Print(vinyl.Evaluate(apps, meta, lib, o).String())
		fmt.Println()
	}

	ranked := vinyl.Rank(vinyl.Aggregate(apps, meta, lib, o), o)
	fillRuntimes(ctx, client, ranked, meta)
	printVinyl(ranked)
	return nil
}

// readRankedLists turns the year-named library playlists into appearances. The
// library is used rather than the JSON files because Apple has already resolved
// each track to a record, which is the fact the ranking needs.
func readRankedLists(ctx context.Context, lib domain.LibraryPort, match string) ([]vinyl.Appearance, int, int, error) {
	existing, err := lib.ExistingPlaylists(ctx)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("list playlists: %w", err)
	}
	names := make([]string, 0, len(existing))
	for name := range existing {
		if strings.Contains(name, match) && yearInName.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var apps []vinyl.Appearance
	first, last := 0, 0
	for _, name := range names {
		year, _ := strconv.Atoi(yearInName.FindString(name))
		refs, err := lib.PlaylistTrackRefs(ctx, existing[name])
		if err != nil {
			return nil, 0, 0, fmt.Errorf("read %q: %w", name, err)
		}
		for i, r := range refs {
			if r.Album == "" {
				continue
			}
			apps = append(apps, vinyl.Appearance{
				Year: year, Rank: i + 1, Size: len(refs),
				Track:    strings.ToLower(r.Title),
				AlbumKey: vinyl.AlbumKey(r.Album, r.Artist),
				Album:    r.Album, Artist: r.Artist,
			})
		}
		if first == 0 || year < first {
			first = year
		}
		if year > last {
			last = year
		}
		fmt.Printf("  %-28s %3d tracks\n", name, len(refs))
	}
	return apps, first, last, nil
}

// albumMetadata resolves each candidate album once, caching to the config dir so
// repeat runs cost no catalog lookups.
func albumMetadata(ctx context.Context, port domain.AlbumPort, apps []vinyl.Appearance,
	lib map[string]int, refresh, cachedOnly bool) (map[string]vinyl.AlbumMeta, error) {
	path, err := config.AlbumIndexPath()
	if err != nil {
		return nil, err
	}
	cache := map[string]vinyl.AlbumMeta{}
	if !refresh {
		if b, err := os.ReadFile(path); err == nil {
			_ = json.Unmarshal(b, &cache)
		}
	}

	need, skipped := candidatesToResolve(apps, cache, lib)
	if cachedOnly {
		// Ranking from what is already known is always possible; a throttled
		// catalog should degrade the shortlist, not block it.
		if len(need) > 0 {
			fmt.Printf("cached-only: %d candidate albums remain unresolved and cannot place\n", len(need))
		}
		return cache, nil
	}
	resolveAlbums(ctx, port, need, cache, path)
	if skipped > 0 {
		fmt.Printf("skipping %d albums with a single track in a single year and no library depth\n", skipped)
	}
	if len(need) > 0 {
		fmt.Printf("resolving %d albums (cached at %s)…\n", len(need), filepath.Base(path))
	}

	if len(need) > 0 {
		if err := saveAt(config.AlbumIndexPath, cache); err != nil {
			return nil, err
		}
	}
	return cache, nil
}

// resolveAlbums fills the cache, strongest candidate first, and never records a
// transient failure as an answer.
func resolveAlbums(ctx context.Context, port domain.AlbumPort, need []albumRef,
	cache map[string]vinyl.AlbumMeta, path string) {
	done, transient := 0, 0
	for _, r := range need {
		if ctx.Err() != nil {
			break
		}
		time.Sleep(albumLookupPace)

		alb, err := port.Album(ctx, r.album, r.artist)
		switch {
		case errors.Is(err, domain.ErrAlbumNotFound):
			// A definitive answer: remember it so we never ask again.
			cache[r.key] = vinyl.AlbumMeta{IsSoundtrack: vinyl.IsSoundtrack(r.album, r.artist)}
		case err != nil:
			// The question could not be asked. Caching this would turn a passing
			// throttle into a permanent verdict.
			transient++
			continue
		default:
			cache[r.key] = vinyl.AlbumMeta{
				TrackCount:    alb.TrackCount,
				IsCompilation: alb.IsCompilation,
				CatalogID:     alb.ID,
				IsSoundtrack:  vinyl.IsSoundtrack(alb.Name, alb.Artist) || vinyl.IsSoundtrack(r.album, r.artist),
			}
		}
		done++
		if done%10 == 0 {
			fmt.Printf("  resolved %d/%d\n", done, len(need))
			_ = saveAt(config.AlbumIndexPath, cache)
		}
	}
	if transient > 0 {
		fmt.Printf("  %d lookups could not be completed (rate limit or network); re-run to finish them\n", transient)
	}
	_ = path
}

// albumLookupPace spaces catalog reads so amp-api does not throttle us into
// its own retry backoff, which is far slower than simply going gently.
const albumLookupPace = 250 * time.Millisecond

// fillRuntimes fetches runtimes for the shortlist only. Runtime decides single
// versus double LP, which is worth a request for twenty records and not worth
// one for five hundred.
func fillRuntimes(ctx context.Context, port domain.AlbumPort, ranked []vinyl.Scored, meta map[string]vinyl.AlbumMeta) {
	changed := false
	for i := range ranked {
		id := ranked[i].CatalogID
		if id == "" || ranked[i].RuntimeMin > 0 {
			continue
		}
		time.Sleep(albumLookupPace)
		ms, err := port.AlbumRuntime(ctx, id)
		if err != nil {
			continue
		}
		ranked[i].RuntimeMin = ms / 60000
		ranked[i].DoubleLP = ranked[i].RuntimeMin > 70
		for k, m := range meta {
			if m.CatalogID == id {
				m.RuntimeMin = ranked[i].RuntimeMin
				meta[k] = m
				changed = true
			}
		}
	}
	if changed {
		_ = saveAt(config.AlbumIndexPath, meta)
	}
}

// albumRef names an album well enough to look it up, and carries the evidence
// that decides whether it is worth looking up at all.
type albumRef struct {
	key           string
	album, artist string
	depth, years  int
	lib           int
}

// candidatesToResolve picks the albums worth a catalog lookup, strongest first,
// and reports how many were skipped.
//
// Two things matter here. Most albums ever touched contributed one track in one
// year and cannot place however generous the scoring, so resolving them is pure
// cost — of 517 albums only 80 can realistically reach a shortlist. And the
// order is not cosmetic: iterating a Go map is randomised, so an interrupted or
// throttled run spends its budget on the long tail and leaves the shortlist
// unresolved. Sorting by evidence means any prefix of the work is the useful
// prefix.
//
// A silent cap reads as "considered everything" when it did not, so the skipped
// count comes back for the caller to say out loud.
func candidatesToResolve(apps []vinyl.Appearance, cache map[string]vinyl.AlbumMeta,
	lib map[string]int) ([]albumRef, int) {
	tracks := map[string]map[string]struct{}{}
	years := map[string]map[int]struct{}{}
	names := map[string]albumRef{}
	for _, a := range apps {
		if tracks[a.AlbumKey] == nil {
			tracks[a.AlbumKey] = map[string]struct{}{}
			years[a.AlbumKey] = map[int]struct{}{}
			names[a.AlbumKey] = albumRef{key: a.AlbumKey, album: a.Album, artist: a.Artist}
		}
		tracks[a.AlbumKey][a.Track] = struct{}{}
		years[a.AlbumKey][a.Year] = struct{}{}
	}

	var need []albumRef
	skipped := 0
	for key, r := range names {
		r.depth, r.years, r.lib = len(tracks[key]), len(years[key]), lib[key]
		if _, ok := cache[key]; ok {
			continue
		}
		// Worth resolving on breadth, on breadth sustained over time, or on
		// independent support in the saved library — any one of which could carry
		// the record onto a shortlist.
		if r.depth >= 3 || (r.depth >= 2 && r.years >= 2) || r.lib >= 5 {
			need = append(need, r)
			continue
		}
		skipped++
	}
	sort.Slice(need, func(i, j int) bool {
		a, b := need[i], need[j]
		if a.depth != b.depth {
			return a.depth > b.depth
		}
		if a.years != b.years {
			return a.years > b.years
		}
		if a.lib != b.lib {
			return a.lib > b.lib
		}
		return a.key < b.key
	})
	return need, skipped
}

// libraryDepth counts distinct tracks per album in the saved Spotify snapshot —
// an independent witness to the ranked lists, which are truncated at 100 and so
// under-report records played steadily rather than obsessively.
func libraryDepth() map[string]int {
	lib, err := loadSnapshot()
	if err != nil {
		return map[string]int{}
	}
	seen := map[string]map[string]struct{}{}
	for _, p := range lib.Playlists {
		for _, t := range p.Tracks {
			if t.Album == "" {
				continue
			}
			artist := ""
			if len(t.Artists) > 0 {
				artist = t.Artists[0]
			}
			k := vinyl.AlbumKey(t.Album, artist)
			if seen[k] == nil {
				seen[k] = map[string]struct{}{}
			}
			seen[k][strings.ToLower(t.Title)] = struct{}{}
		}
	}
	out := make(map[string]int, len(seen))
	for k, v := range seen {
		out[k] = len(v)
	}
	return out
}

func printVinyl(ranked []vinyl.Scored) {
	if len(ranked) == 0 {
		fmt.Println("\nno albums qualified — try --include-soundtracks or a lower --min-tracks")
		return
	}
	fmt.Printf("\n%-3s %-36s %-20s %5s %6s %5s %5s %5s %s\n",
		"#", "album", "artist", "score", "loved", "years", "seen", "est", "format")
	fmt.Println(strings.Repeat("-", 108))
	for i, s := range ranked {
		format := fmt.Sprintf("%dm", s.RuntimeMin)
		if s.DoubleLP {
			format += " 2LP"
		}
		fmt.Printf("%-3d %-36s %-20s %5.3f %3d/%-2d %5d %4.0f%% %4.0f%% %s\n",
			i+1, truncate(s.Album, 36), truncate(s.Artist, 20), s.Score,
			s.LovedTracks, s.TrackCount, len(s.Years), s.Observed*100, s.Coverage*100, format)
	}
	fmt.Println("\nloved/seen = tracks of the record that reached your yearly lists")
	fmt.Println("est        = share after crediting tracks that likely played just below the top-100 cutoff")
}
