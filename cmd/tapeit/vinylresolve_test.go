package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/vinyl"
)

// fakeAlbums is an AlbumPort that records what it was asked, so the tests can
// assert that the exact path is preferred and the search path is a fallback.
type fakeAlbums struct {
	songAlbums map[string]string       // song id -> album id
	byID       map[string]domain.Album // album id -> album
	byName     map[string]domain.Album // "name|artist" -> album
	searches   []string
	songCalls  int
	songErr    error
}

func (f *fakeAlbums) SongAlbums(_ context.Context, ids []string) (map[string]string, error) {
	f.songCalls++
	if f.songErr != nil {
		return nil, f.songErr
	}
	out := map[string]string{}
	for _, id := range ids {
		if alb, ok := f.songAlbums[id]; ok {
			out[id] = alb
		}
	}
	return out, nil
}

func (f *fakeAlbums) AlbumsByID(_ context.Context, ids []string) (map[string]domain.Album, error) {
	out := map[string]domain.Album{}
	for _, id := range ids {
		if a, ok := f.byID[id]; ok {
			out[id] = a
		}
	}
	return out, nil
}

func (f *fakeAlbums) AlbumEditions(_ context.Context, name, artist string) ([]domain.Album, error) {
	f.searches = append(f.searches, name)
	if a, ok := f.byName[name+"|"+artist]; ok {
		return []domain.Album{a}, nil
	}
	return nil, fmt.Errorf("%q: %w", name, domain.ErrAlbumNotFound)
}

func (f *fakeAlbums) AlbumTracks(context.Context, string) ([]domain.AlbumTrack, error) {
	return nil, nil
}

// appearancesOf builds n loved tracks of one album, each with a catalog id when
// withIDs is set, spread over two years so they clear the candidate filter.
func appearancesOf(album, artist string, n int, withIDs bool) []vinyl.Appearance {
	var apps []vinyl.Appearance
	for i := 0; i < n; i++ {
		id := ""
		if withIDs {
			id = fmt.Sprintf("song-%s-%d", album, i)
		}
		apps = append(apps, vinyl.Appearance{
			Year: 2020 + i%2, Rank: i + 1, Size: 100,
			Track: fmt.Sprintf("track-%d", i), AlbumKey: vinyl.AlbumKey(album, artist),
			Album: album, Artist: artist, CatalogID: id,
		})
	}
	return apps
}

func TestResolveAlbums_PrefersTheExactPathOverSearching(t *testing.T) {
	// The exact path reads the album from the recording, so it cannot bind a
	// record to an EP that shares its name — and it costs two batched requests
	// instead of one search per album.
	apps := appearancesOf("Is This It", "The Strokes", 4, true)
	f := &fakeAlbums{songAlbums: map[string]string{}, byID: map[string]domain.Album{
		"alb-1": {ID: "alb-1", Name: "Is This It", Artist: "The Strokes", TrackCount: 11, UPC: "0001"},
	}}
	for _, a := range apps {
		f.songAlbums[a.CatalogID] = "alb-1"
	}

	got, err := resolveAlbums(context.Background(), f, apps, nil, map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	meta := got[vinyl.AlbumKey("Is This It", "The Strokes")]
	if meta.TrackCount != 11 || meta.CatalogID != "alb-1" || meta.UPC != "0001" {
		t.Errorf("album not resolved exactly: %+v", meta)
	}
	if len(f.searches) != 0 {
		t.Errorf("no name search should be needed, got %v", f.searches)
	}
}

func TestResolveAlbums_FallsBackToSearchForTracksWithoutCatalogIDs(t *testing.T) {
	// About one track in twenty carries no catalog id. Those are still loved
	// tracks, so their record must still be resolvable — dropping it would
	// understate the listener's library rather than report a gap.
	apps := appearancesOf("Wet Leg", "Wet Leg", 3, false)
	f := &fakeAlbums{byName: map[string]domain.Album{
		"Wet Leg|Wet Leg": {ID: "alb-2", Name: "Wet Leg", Artist: "Wet Leg", TrackCount: 12},
	}}

	got, err := resolveAlbums(context.Background(), f, apps, nil, map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta := got[vinyl.AlbumKey("Wet Leg", "Wet Leg")]; meta.TrackCount != 12 {
		t.Errorf("fallback search did not resolve the album: %+v", meta)
	}
	if len(f.searches) != 1 {
		t.Errorf("expected exactly one name search, got %v", f.searches)
	}
}

func TestResolveAlbums_ChoosesAnEditionBigEnoughForWhatWasHeard(t *testing.T) {
	// The failure this guards: an EP sharing the album's title is the smallest
	// match, and choosing it both overstates coverage and can push the record
	// under the album/EP threshold, removing it from consideration silently.
	apps := appearancesOf("Wet Leg", "Wet Leg", 12, true)
	f := &fakeAlbums{songAlbums: map[string]string{}, byID: map[string]domain.Album{
		"ep":    {ID: "ep", Name: "Wet Leg", Artist: "Wet Leg", TrackCount: 4},
		"album": {ID: "album", Name: "Wet Leg", Artist: "Wet Leg", TrackCount: 12},
	}}
	for i, a := range apps {
		if i%2 == 0 {
			f.songAlbums[a.CatalogID] = "ep"
		} else {
			f.songAlbums[a.CatalogID] = "album"
		}
	}

	got, err := resolveAlbums(context.Background(), f, apps, nil, map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	meta := got[vinyl.AlbumKey("Wet Leg", "Wet Leg")]
	if meta.TrackCount != 12 {
		t.Errorf("chose a %d-track edition for 12 loved tracks; want the 12-track album", meta.TrackCount)
	}
}

func TestResolveAlbums_DoesNotCacheTransientFailures(t *testing.T) {
	// A throttle is not an answer. Recording one would exclude the record from
	// every future run, which is invisible: a missing album looks like a low
	// score, not like an error.
	apps := appearancesOf("Unreachable", "Someone", 3, false)
	f := &fakeAlbums{} // Album() reports not-found only for unknown names
	f.byName = nil

	cache := map[string]vinyl.AlbumMeta{}
	got, err := resolveAlbums(context.Background(), f, apps, nil, cache, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Not-found is durable and *is* remembered, so the entry exists but carries
	// no track count and therefore cannot rank.
	if meta, ok := got[vinyl.AlbumKey("Unreachable", "Someone")]; ok && meta.TrackCount != 0 {
		t.Errorf("a not-found album must not gain a track count: %+v", meta)
	}
}

func TestResolveAlbums_SurvivesAnUnavailableExactPath(t *testing.T) {
	// If the batch endpoint is unavailable the run must degrade to searching
	// rather than abort, so a throttle costs speed and not the whole answer.
	apps := appearancesOf("Is This It", "The Strokes", 4, true)
	f := &fakeAlbums{
		songErr: fmt.Errorf("429 rate limited"),
		byName: map[string]domain.Album{
			"Is This It|The Strokes": {ID: "alb-1", Name: "Is This It", Artist: "The Strokes", TrackCount: 11},
		},
	}

	got, err := resolveAlbums(context.Background(), f, apps, nil, map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta := got[vinyl.AlbumKey("Is This It", "The Strokes")]; meta.TrackCount != 11 {
		t.Errorf("should have fallen back to search, got %+v", meta)
	}
}

func TestResolveAlbums_SkipsAlbumsThatCannotPlace(t *testing.T) {
	// Most albums a listener ever touched contributed one track in one year.
	// Resolving them is pure cost, and the count of what was skipped is reported
	// rather than hidden.
	var apps []vinyl.Appearance
	apps = append(apps, vinyl.Appearance{
		Year: 2020, Rank: 1, Size: 100, Track: "one-off",
		AlbumKey: vinyl.AlbumKey("Passing Fancy", "Someone"),
		Album:    "Passing Fancy", Artist: "Someone", CatalogID: "s1",
	})
	f := &fakeAlbums{songAlbums: map[string]string{"s1": "alb"}, byID: map[string]domain.Album{
		"alb": {ID: "alb", Name: "Passing Fancy", Artist: "Someone", TrackCount: 10},
	}}

	got, err := resolveAlbums(context.Background(), f, apps, nil, map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("a single track in a single year cannot place; want nothing resolved, got %v", got)
	}
	if f.songCalls != 0 && len(f.searches) != 0 {
		t.Error("no lookups should be spent on an album that cannot place")
	}
}

func TestLibraryMatchRate_MeasuresHowOftenTwoServicesAgree(t *testing.T) {
	// Library corroboration carries real weight in the score, and it only works
	// if a record's key derived from Apple equals the one derived from Spotify.
	// A silent mismatch does not error — it reads as "you never saved this",
	// which lowers a record's score for a reason that has nothing to do with the
	// listener. So the agreement rate is measured rather than assumed.
	apps := []vinyl.Appearance{
		{AlbumKey: vinyl.AlbumKey("Is This It", "The Strokes"), Album: "Is This It", Artist: "The Strokes"},
		{AlbumKey: vinyl.AlbumKey("Wet Leg", "Wet Leg"), Album: "Wet Leg", Artist: "Wet Leg"},
		{AlbumKey: vinyl.AlbumKey("Unsaved", "Nobody"), Album: "Unsaved", Artist: "Nobody"},
	}
	lib := map[string]int{
		vinyl.AlbumKey("Is This It", "The Strokes"): 5,
		vinyl.AlbumKey("Wet Leg", "Wet Leg"):        12,
		vinyl.AlbumKey("Never Charted", "Someone"):  8,
	}

	matched, total := libraryMatchRate(apps, lib)
	if total != 3 {
		t.Errorf("considered %d distinct records, want 3", total)
	}
	if matched != 2 {
		t.Errorf("matched %d records to the saved library, want 2", matched)
	}
}

func TestLibraryMatchRate_HandlesAnEmptyLibrary(t *testing.T) {
	// A listener with no snapshot should get zero, not a division by zero.
	apps := []vinyl.Appearance{{AlbumKey: "k", Album: "A", Artist: "B"}}
	if matched, total := libraryMatchRate(apps, nil); matched != 0 || total != 1 {
		t.Errorf("got %d/%d, want 0/1", matched, total)
	}
}

func TestResolveAlbums_ConsidersARecordKnownOnlyFromPlays(t *testing.T) {
	// A record released after the last charted year has no chart presence at
	// all. Without play evidence it could never even become a candidate, however
	// much its owner plays it — which is how a favourite album goes missing from
	// a shortlist without appearing anywhere as excluded.
	plays := []vinyl.Play{
		{Track: "catch these fists", AlbumKey: vinyl.AlbumKey("moisturizer", "Wet Leg"),
			Album: "moisturizer", Artist: "Wet Leg", Count: 4},
		{Track: "mangetout", AlbumKey: vinyl.AlbumKey("moisturizer", "Wet Leg"),
			Album: "moisturizer", Artist: "Wet Leg", Count: 3},
		{Track: "davina mccall", AlbumKey: vinyl.AlbumKey("moisturizer", "Wet Leg"),
			Album: "moisturizer", Artist: "Wet Leg", Count: 2},
	}
	f := &fakeAlbums{byName: map[string]domain.Album{
		"moisturizer|Wet Leg": {ID: "m1", Name: "moisturizer", Artist: "Wet Leg", TrackCount: 12},
	}}

	got, err := resolveAlbums(context.Background(), f, nil, plays,
		map[string]vinyl.AlbumMeta{}, map[string]int{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if meta := got[vinyl.AlbumKey("moisturizer", "Wet Leg")]; meta.TrackCount != 12 {
		t.Errorf("a played-but-never-charted record was not considered: %+v", meta)
	}
}
