package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/vinyl"
)

// albumLookupPace spaces catalog reads. Apple answers a burst with a 429 and the
// client then backs off through eight attempts, so an unpaced loop does not fail
// — it silently crawls. Going gently costs less than the backoff it avoids.
const albumLookupPace = 250 * time.Millisecond

// resolveAlbums establishes, for every album a listener's tracks belong to, the
// edition that record should be judged by.
//
// It prefers the exact path: a library track carries the catalog id of its
// recording, and a recording knows its album, so the album is read from the data
// rather than guessed from a name. That is both faster — two batched requests
// per twenty-five tracks instead of one search per album — and safer, since a
// name search can bind a record to an EP that happens to share its title.
//
// Tracks the library has no catalog id for (about one in twenty) fall back to
// searching by name. They are a fallback rather than a filter: a track without
// an id is still a track the listener loves, and dropping it would quietly
// understate the record.
func resolveAlbums(ctx context.Context, port domain.AlbumPort, apps []vinyl.Appearance,
	plays []vinyl.Play, cache map[string]vinyl.AlbumMeta, lib map[string]int,
	pace time.Duration) (map[string]vinyl.AlbumMeta, error) {
	need, skipped := candidatesToResolve(apps, plays, cache, lib)
	if skipped > 0 {
		fmt.Printf("skipping %d albums with a single track in a single year and no library depth\n", skipped)
	}
	if len(need) == 0 {
		return cache, nil
	}
	fmt.Printf("resolving %d albums…\n", len(need))

	wanted := make(map[string]albumRef, len(need))
	for _, r := range need {
		wanted[r.key] = r
	}
	editions := exactEditions(ctx, port, apps, plays, wanted)
	searchEditions(ctx, port, wanted, editions, cache, pace)

	loved := lovedCounts(apps, plays)
	for key, eds := range editions {
		chosen, ok := vinyl.ChooseEdition(eds, loved[key])
		if !ok {
			continue
		}
		cache[key] = vinyl.AlbumMeta{
			Name:          chosen.Name,
			Artist:        chosen.Artist,
			TrackCount:    chosen.TrackCount,
			RuntimeMin:    chosen.RuntimeMin,
			CatalogID:     chosen.ID,
			UPC:           chosen.UPC,
			IsCompilation: chosen.IsCompilation,
			Genres:        chosen.Genres,
			IsSoundtrack:  vinyl.IsSoundtrack(chosen.Name, chosen.Artist, chosen.Genres),
		}
	}
	return cache, nil
}

// exactEditions reads albums straight from the recordings, which is both the
// cheap path and the only one that cannot pick the wrong record.
func exactEditions(ctx context.Context, port domain.AlbumPort, apps []vinyl.Appearance,
	_ []vinyl.Play, wanted map[string]albumRef) map[string][]vinyl.Edition {
	songIDs := make([]string, 0, len(apps))
	seen := map[string]struct{}{}
	for _, a := range apps {
		if a.CatalogID == "" {
			continue
		}
		if _, dup := seen[a.CatalogID]; dup {
			continue
		}
		seen[a.CatalogID] = struct{}{}
		songIDs = append(songIDs, a.CatalogID)
	}
	sort.Strings(songIDs) // stable request order, so runs are reproducible

	editions := map[string][]vinyl.Edition{}
	if len(songIDs) == 0 {
		return editions
	}
	songAlbum, err := port.SongAlbums(ctx, songIDs)
	if err != nil {
		fmt.Printf("  exact album lookup unavailable (%v); falling back to search\n", err)
		return editions
	}
	albumIDs := make([]string, 0, len(songAlbum))
	uniq := map[string]struct{}{}
	for _, id := range songAlbum {
		if _, dup := uniq[id]; !dup {
			uniq[id] = struct{}{}
			albumIDs = append(albumIDs, id)
		}
	}
	sort.Strings(albumIDs)

	albums, err := port.AlbumsByID(ctx, albumIDs)
	if err != nil {
		fmt.Printf("  album details unavailable (%v); falling back to search\n", err)
		return editions
	}
	for _, alb := range albums {
		key := domain.AlbumKey(alb.Name, alb.Artist)
		if _, want := wanted[key]; !want {
			continue
		}
		editions[key] = append(editions[key], toEdition(alb))
	}
	fmt.Printf("  %d albums resolved exactly from %d recordings\n", len(editions), len(songIDs))
	return editions
}

// searchEditions fills the gaps left by the exact pass, by name.
func searchEditions(ctx context.Context, port domain.AlbumPort, wanted map[string]albumRef,
	editions map[string][]vinyl.Edition, cache map[string]vinyl.AlbumMeta, pace time.Duration) {
	var missing []albumRef
	for key, r := range wanted {
		if len(editions[key]) == 0 {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].moreEvidenceThan(missing[j]) })
	fmt.Printf("  %d albums need a name search (no catalog id on their tracks)\n", len(missing))

	transient := 0
	for _, r := range missing {
		if ctx.Err() != nil {
			break
		}
		time.Sleep(pace)
		alb, err := port.Album(ctx, r.album, r.artist)
		switch {
		case errors.Is(err, domain.ErrAlbumNotFound):
			// A durable answer: remember it rather than ask again every run.
			cache[r.key] = vinyl.AlbumMeta{
				Name: r.album, Artist: r.artist,
				IsSoundtrack: vinyl.IsSoundtrack(r.album, r.artist, nil),
			}
		case err != nil:
			// Could not ask. Recording this would turn a passing throttle into a
			// permanent verdict, so leave it for the next run.
			transient++
		default:
			editions[r.key] = append(editions[r.key], toEdition(alb))
		}
	}
	if transient > 0 {
		fmt.Printf("  %d lookups could not be completed (rate limit or network); re-run to finish them\n", transient)
	}
}

func toEdition(a domain.Album) vinyl.Edition {
	return vinyl.Edition{
		ID: a.ID, Name: a.Name, Artist: a.Artist,
		TrackCount: a.TrackCount, RuntimeMin: a.RuntimeMS / 60000,
		UPC: a.UPC, Genres: a.Genres, IsCompilation: a.IsCompilation,
	}
}

// lovedCounts is how many distinct tracks of each record the listener loves. It
// is the lower bound that keeps ChooseEdition from picking an edition too small
// to be the one they were listening to.
func lovedCounts(apps []vinyl.Appearance, plays []vinyl.Play) map[string]int {
	tracks := map[string]map[string]struct{}{}
	add := func(key, track string) {
		if key == "" {
			return
		}
		if tracks[key] == nil {
			tracks[key] = map[string]struct{}{}
		}
		tracks[key][track] = struct{}{}
	}
	for _, a := range apps {
		add(a.AlbumKey, a.Track)
	}
	for _, p := range plays {
		add(p.AlbumKey, p.Track)
	}
	out := make(map[string]int, len(tracks))
	for k, v := range tracks {
		out[k] = len(v)
	}
	return out
}

// libraryMatchRate reports how many of the records a listener charted are also
// found in their separately saved library, by key.
//
// The saved library carries real weight in the score, and it reaches the model
// only through a key derived independently on each side: from Apple for the
// charts, from Spotify for the library. A key that disagrees does not raise an
// error — it reads as "you never saved this", which lowers a record's score for
// a reason that has nothing to do with the listener.
//
// That makes it exactly the kind of assumption worth measuring rather than
// trusting. A rate far below what a listener would recognise as true is a sign
// the two sides have stopped agreeing about what one album is.
func libraryMatchRate(apps []vinyl.Appearance, lib map[string]int) (matched, total int) {
	seen := map[string]struct{}{}
	for _, a := range apps {
		if a.AlbumKey == "" {
			continue
		}
		if _, dup := seen[a.AlbumKey]; dup {
			continue
		}
		seen[a.AlbumKey] = struct{}{}
		total++
		if lib[a.AlbumKey] > 0 {
			matched++
		}
	}
	return matched, total
}
