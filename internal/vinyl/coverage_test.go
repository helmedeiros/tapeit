package vinyl

import "testing"

func TestTracksOnEdition_CountsOnlyWhatTheRecordActuallyHolds(t *testing.T) {
	// The correction this exists for: loved tracks are gathered across every
	// edition, but you buy one pressing. Two of these are deluxe-only, so the
	// standard LP delivers three of them, not five — and for a purchase decision
	// that difference points the wrong way.
	loved := []string{"Sun", "Moon", "Stars", "Bonus One", "Bonus Two"}
	edition := []string{"Sun", "Moon", "Stars", "Rain", "Wind"}

	if got := TracksOnEdition(loved, edition); got != 3 {
		t.Errorf("counted %d loved tracks on the record, want 3", got)
	}
}

func TestTracksOnEdition_IgnoresCaseAndPunctuation(t *testing.T) {
	// The same recording is spelled differently by the library and the catalog,
	// and a spelling difference must not read as "not on this record".
	loved := []string{"Don't Go Yet", "I WANNA BE YOUR SLAVE", "Señorita"}
	edition := []string{"Dont Go Yet", "I Wanna Be Your Slave", "Senorita"}

	if got := TracksOnEdition(loved, edition); got != 3 {
		t.Errorf("counted %d, want all 3 matched despite spelling", got)
	}
}

func TestTracksOnEdition_MatchesAcrossVersionSuffixes(t *testing.T) {
	// A listener's copy may carry a qualifier the pressing does not, or the
	// reverse. Both name the same performance.
	loved := []string{"Song 2 - 2012 Remaster", "Humility"}
	edition := []string{"Song 2", "Humility (feat. George Benson)"}

	if got := TracksOnEdition(loved, edition); got != 2 {
		t.Errorf("counted %d, want 2 across version suffixes", got)
	}
}

func TestTracksOnEdition_DoesNotMatchDifferentSongs(t *testing.T) {
	// Over-matching is the dangerous direction: it would credit a record with
	// songs it does not contain.
	loved := []string{"Yellow", "Clocks"}
	edition := []string{"Yellow Submarine", "Clockwork"}

	if got := TracksOnEdition(loved, edition); got != 0 {
		t.Errorf("counted %d, want 0 — these are different songs", got)
	}
}

func TestTracksOnEdition_CountsEachLovedTrackOnce(t *testing.T) {
	// A record listing the same title twice (a reprise, a hidden track) must not
	// inflate coverage beyond what was loved.
	loved := []string{"Intro"}
	edition := []string{"Intro", "Intro", "Outro"}

	if got := TracksOnEdition(loved, edition); got != 1 {
		t.Errorf("counted %d, want 1", got)
	}
}

func TestTracksOnEdition_EmptyInputs(t *testing.T) {
	if got := TracksOnEdition(nil, []string{"A"}); got != 0 {
		t.Errorf("nothing loved means nothing on the record, got %d", got)
	}
	// An unknown track listing must report zero rather than guess, so callers can
	// tell "we did not check" from "none of them are on it".
	if got := TracksOnEdition([]string{"A"}, nil); got != 0 {
		t.Errorf("unknown listing must not be treated as a match, got %d", got)
	}
}

func TestFoldDiacritics_CoversTheAccentsThatAppearInTitles(t *testing.T) {
	cases := map[string]string{
		"Señorita":        "senorita",
		"Désolé":          "desole",
		"Você Não Presta": "voce nao presta",
		"Ofertório":       "ofertorio",
		"Måneskin":        "maneskin",
		"Cadê Teu Suin?":  "cade teu suin?",
		"plain":           "plain",
	}
	for in, want := range cases {
		if got := foldDiacritics(in); got != want {
			t.Errorf("foldDiacritics(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScore_UsesTheVerifiedPressingCountWhenChecked(t *testing.T) {
	// Before checking, coverage assumes every loved track is on the record. After
	// checking, it must use what the pressing actually holds — and the difference
	// points the wrong way for a purchase, so it has to be the checked figure
	// that wins.
	e := album("Deluxe-heavy", "A", 8, 11, 2022, 2024)
	unchecked := Score(e, 8, window())

	e.PressingChecked, e.PressingTracks = true, 5
	checked := Score(e, 8, window())

	if checked.Observed >= unchecked.Observed {
		t.Errorf("verified coverage should be lower here: %.3f vs unverified %.3f",
			checked.Observed, unchecked.Observed)
	}
	if got, want := checked.Observed, 5.0/11.0; got != want {
		t.Errorf("observed = %.4f, want %.4f (5 of 11 actually on the record)", got, want)
	}
}

func TestScore_VerifiedPressingOfZeroIsHonoured(t *testing.T) {
	// A record where none of the loved tracks are on this pressing is a real
	// answer, not a missing one, and must not fall back to the optimistic count.
	e := album("Wrong edition", "A", 4, 11, 2022, 2024)
	e.PressingChecked, e.PressingTracks = true, 0

	if got := Score(e, 4, window()); got.Observed != 0 {
		t.Errorf("observed = %.3f, want 0 — none of the loved tracks are on it", got.Observed)
	}
}

func TestRefine_RescoresAndReordersOnVerifiedCoverage(t *testing.T) {
	// Verification can change the answer, not just the numbers: a record whose
	// loved tracks turn out to be deluxe-only should fall behind one whose
	// tracks are all on the standard pressing.
	deluxeHeavy := album("Deluxe Heavy", "A", 8, 11, 2022, 2024)
	deluxeHeavy.LovedTitles = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	honest := album("Honest", "B", 6, 11, 2022, 2024)
	honest.LovedTitles = []string{"a", "b", "c", "d", "e", "f"}

	o := window()
	ranked := Rank([]Evidence{deluxeHeavy, honest}, o)
	if ranked[0].Album != "Deluxe Heavy" {
		t.Fatalf("precondition: expected the deeper album first, got %q", ranked[0].Album)
	}

	// Only two of the first record's loved tracks are on the pressing you buy.
	pressings := map[string][]string{
		"Deluxe Heavy": {"a", "b", "x", "y", "z"},
		"Honest":       {"a", "b", "c", "d", "e", "f"},
	}
	refined := Refine(ranked, func(s Scored) ([]string, bool) {
		p, ok := pressings[s.Album]
		return p, ok
	}, o)

	if refined[0].Album != "Honest" {
		t.Errorf("verified coverage should reorder the shortlist, got %q first", refined[0].Album)
	}
	for _, s := range refined {
		if !s.PressingChecked {
			t.Errorf("%q was not marked as checked", s.Album)
		}
	}
}

func TestRefine_LeavesUncheckableRecordsAlone(t *testing.T) {
	// A pressing we could not read is unknown, not empty. Treating it as empty
	// would zero a good record's coverage on a network failure.
	e := album("Unreadable", "A", 8, 11, 2022, 2024)
	e.LovedTitles = []string{"a", "b"}
	o := window()
	before := Rank([]Evidence{e}, o)

	after := Refine(before, func(Scored) ([]string, bool) { return nil, false }, o)
	if after[0].PressingChecked {
		t.Error("an unreadable pressing must not be recorded as checked")
	}
	if after[0].Score != before[0].Score {
		t.Errorf("score changed on an unreadable pressing: %.4f -> %.4f", before[0].Score, after[0].Score)
	}
}
