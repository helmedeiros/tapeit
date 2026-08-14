package vinyl

import "testing"

func TestIsSoundtrack_TrustsGenreOverTitle(t *testing.T) {
	// The catalog says what a record is. Reading that is safer than inferring it
	// from the title, which is guesswork dressed as a rule.
	for _, genres := range [][]string{
		{"Soundtracks", "Musik"},
		{"Filmmusik"},
		{"Musicals", "Soundtracks", "Hip-Hop/Rap"},
		{"Original Score"},
	} {
		if !IsSoundtrack("Some Record", "Someone", genres) {
			t.Errorf("genres %v should identify a soundtrack", genres)
		}
	}
}

func TestIsSoundtrack_DoesNotConvictARecordOnItsTitleAlone(t *testing.T) {
	// The failure this guards: a record whose title merely reads like a
	// soundtrack is excluded from consideration entirely, and — because
	// exclusions were silent — never appears anywhere for the listener to
	// question. "Music from Big Pink" is a rock album.
	cases := []struct {
		album, artist string
		genres        []string
	}{
		{"Music from Big Pink", "The Band", []string{"Rock", "Americana", "Folk-Rock"}},
		{"Original Pirate Material", "The Streets", []string{"Hip-Hop/Rap"}},
		{"Songs from the Big Chair", "Tears For Fears", []string{"Pop", "Rock"}},
		{"The Original Sessions", "Someone", []string{"Jazz"}},
	}
	for _, c := range cases {
		if IsSoundtrack(c.album, c.artist, c.genres) {
			t.Errorf("%q by %q is not a soundtrack (genres %v)", c.album, c.artist, c.genres)
		}
	}
}

func TestIsSoundtrack_FallsBackToTitleWhenGenresAreUnknown(t *testing.T) {
	// Genres are missing for albums that could not be resolved. A title that
	// states outright what the record accompanies is still worth acting on, but
	// only the unambiguous forms — the ones that name a film or a production.
	for _, album := range []string{
		"Moana (Original Motion Picture Soundtrack)",
		"Trolls (Original Motion Picture Soundtrack)",
		"The Lego Ninjago Movie (Original Motion Picture Soundtrack)",
		"Sing 2 (Original Motion Picture Soundtrack)",
		"Hamilton (Original Broadway Cast Recording)",
	} {
		if !IsSoundtrack(album, "Various Artists", nil) {
			t.Errorf("%q states plainly that it is a soundtrack", album)
		}
	}
	for _, album := range []string{
		"Music from Big Pink",
		"Original Pirate Material",
		"Songs from the Big Chair",
	} {
		if IsSoundtrack(album, "Someone", nil) {
			t.Errorf("%q must not be convicted on an ambiguous phrase", album)
		}
	}
}

func TestIsSoundtrack_GenreEvidenceOverridesAnInnocentTitle(t *testing.T) {
	// A score can be titled like an ordinary record. The catalog still knows.
	if !IsSoundtrack("Alien Coast", "Someone", []string{"Soundtracks"}) {
		t.Error("genre evidence should stand on its own")
	}
}

func TestRank_ReportsWhatItExcludedAndWhy(t *testing.T) {
	// An exclusion is consequential: the record does not rank lower, it never
	// appears. Reporting it is what lets a listener notice a wrong call and
	// override it, instead of wondering where a favourite went.
	ev := []Evidence{
		album("Is This It", "The Strokes", 9, 11, 2019, 2022),
		func() Evidence { e := album("Moana", "Various", 8, 12, 2018, 2019); e.IsSoundtrack = true; return e }(),
		func() Evidence { e := album("Some Single", "A", 2, 2, 2020, 2021); e.RuntimeMin = 6; return e }(),
		album("One Track", "B", 1, 12, 2019, 2021),
	}

	ranked, excluded := RankWithExclusions(ev, window())
	if len(ranked) != 1 || ranked[0].Album != "Is This It" {
		t.Fatalf("expected one ranked record, got %d: %+v", len(ranked), ranked)
	}
	reasons := map[string]string{}
	for _, e := range excluded {
		reasons[e.Album] = e.Excluded
	}
	for album, want := range map[string]string{
		"Moana":       "soundtrack",
		"Some Single": "single or EP",
		"One Track":   "only one loved track",
	} {
		if reasons[album] != want {
			t.Errorf("%q excluded as %q, want %q", album, reasons[album], want)
		}
	}
}

func TestRank_StillReturnsOnlyTheRanked(t *testing.T) {
	// The existing entry point must keep its shape; reporting is additive.
	ev := []Evidence{
		album("Kept", "A", 9, 11, 2019, 2022),
		func() Evidence { e := album("Dropped", "B", 8, 12, 2018); e.IsSoundtrack = true; return e }(),
	}
	if got := Rank(ev, window()); len(got) != 1 || got[0].Album != "Kept" {
		t.Errorf("Rank should return only ranked records, got %+v", got)
	}
}
