package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
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
	minLoved := fs.Int("min-loved", 2, "fewest of a record's tracks that must have reached your lists")
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
	o.MinLovedTracks = *minLoved

	if *evaluate {
		fmt.Println()
		fmt.Print(vinyl.Evaluate(apps, meta, lib, o).String())
		fmt.Println()
	}

	ranked := vinyl.Rank(vinyl.Aggregate(apps, meta, lib, o), o)
	ranked = refineShortlist(ctx, client, ranked, meta, o)
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
				Track:     strings.ToLower(r.Title),
				AlbumKey:  vinyl.AlbumKey(r.Album, r.Artist),
				Album:     r.Album,
				Artist:    r.Artist,
				CatalogID: r.CatalogID,
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
	if cachedOnly {
		// Ranking from what is already known is always possible; a throttled
		// catalog should degrade the shortlist, not block it.
		if need, _ := candidatesToResolve(apps, cache, lib); len(need) > 0 {
			fmt.Printf("cached-only: %d candidate albums remain unresolved and cannot place\n", len(need))
		}
		return cache, nil
	}

	before := len(cache)
	cache, err = resolveAlbums(ctx, port, apps, cache, lib, albumLookupPace)
	if err != nil {
		return nil, err
	}
	if len(cache) != before {
		if err := saveAt(config.AlbumIndexPath, cache); err != nil {
			return nil, err
		}
	}
	return cache, nil
}

// refineShortlist reads each shortlisted record's track listing and re-scores
// on what that pressing actually holds.
//
// One request per record answers both questions that decide a purchase: how
// long it runs, and which of the listener's loved tracks are on this pressing
// rather than on some other edition. Only the shortlist is worth that request.
func refineShortlist(ctx context.Context, port domain.AlbumPort, ranked []vinyl.Scored,
	meta map[string]vinyl.AlbumMeta, o vinyl.Options) []vinyl.Scored {
	runtimes := map[string]int{}
	refined := vinyl.Refine(ranked, func(s vinyl.Scored) ([]string, bool) {
		if s.CatalogID == "" || ctx.Err() != nil {
			return nil, false
		}
		time.Sleep(albumLookupPace)
		tracks, err := port.AlbumTracks(ctx, s.CatalogID)
		if err != nil {
			// Unknown, not empty: leave the record as it was rather than zero it.
			return nil, false
		}
		titles := make([]string, 0, len(tracks))
		total := 0
		for _, t := range tracks {
			titles = append(titles, t.Title)
			total += t.DurationMS
		}
		runtimes[s.CatalogID] = total / 60000
		return titles, true
	}, o)

	changed := false
	for i := range refined {
		if mins, ok := runtimes[refined[i].CatalogID]; ok {
			refined[i].RuntimeMin = mins
			refined[i].DoubleLP = mins > vinyl.DoubleLPMinutes
		}
	}
	for k, m := range meta {
		if mins, ok := runtimes[m.CatalogID]; ok && m.RuntimeMin != mins {
			m.RuntimeMin = mins
			meta[k] = m
			changed = true
		}
	}
	if changed {
		_ = saveAt(config.AlbumIndexPath, meta)
	}
	return refined
}

// albumRef names an album well enough to look it up, and carries the evidence
// that decides whether it is worth looking up at all.
type albumRef struct {
	key           string
	album, artist string
	depth, years  int
	lib           int
}

// moreEvidenceThan orders candidates so the records most likely to place are
// resolved first. Order matters because a run can be cut short by a throttle or
// an interrupt: resolving in evidence order means any prefix of the work is the
// useful prefix, rather than whatever a map happened to yield.
func (r albumRef) moreEvidenceThan(o albumRef) bool {
	if r.depth != o.depth {
		return r.depth > o.depth
	}
	if r.years != o.years {
		return r.years > o.years
	}
	if r.lib != o.lib {
		return r.lib > o.lib
	}
	return r.key < o.key
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
	sort.Slice(need, func(i, j int) bool { return need[i].moreEvidenceThan(need[j]) })
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
