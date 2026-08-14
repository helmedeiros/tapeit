package vinyl

import "testing"

func TestChooseEdition_PrefersTheSmallestThatFitsWhatWasHeard(t *testing.T) {
	// Coverage divides by the edition's track count, so this choice sets the
	// denominator of the whole model. The standard pressing is also what one
	// actually buys, so among the editions that fit, smallest wins.
	eds := []Edition{
		{ID: "deluxe", Name: "Future Nostalgia (Deluxe)", TrackCount: 19},
		{ID: "standard", Name: "Future Nostalgia", TrackCount: 11},
	}
	got, ok := ChooseEdition(eds, 7)
	if !ok || got.ID != "standard" {
		t.Errorf("want the 11-track standard edition, got %+v (ok=%v)", got, ok)
	}
}

func TestChooseEdition_RejectsEditionsTooSmallToHoldWhatWasHeard(t *testing.T) {
	// The observed count is a hard lower bound: you cannot love twelve tracks
	// from a ten-track record. Choosing the smaller edition anyway would both
	// overstate coverage and, worse, drop the record below a minimum-track
	// filter — removing it from consideration without ever saying so.
	eds := []Edition{
		{ID: "ep", Name: "Wet Leg EP", TrackCount: 4},
		{ID: "album", Name: "Wet Leg", TrackCount: 12},
	}
	got, ok := ChooseEdition(eds, 12)
	if !ok || got.ID != "album" {
		t.Errorf("an edition smaller than the observed count cannot be the one heard: got %+v", got)
	}
}

func TestChooseEdition_FallsBackToLargestWhenNoneFit(t *testing.T) {
	// Loved counts can exceed every known edition when a listener's tracks span
	// editions. The largest is then the closest honest denominator; silently
	// returning nothing would drop a real record.
	eds := []Edition{
		{ID: "a", TrackCount: 10},
		{ID: "b", TrackCount: 12},
	}
	got, ok := ChooseEdition(eds, 15)
	if !ok || got.ID != "b" {
		t.Errorf("want the largest edition as the fallback, got %+v (ok=%v)", got, ok)
	}
}

func TestChooseEdition_IgnoresEditionsWithNoTrackCount(t *testing.T) {
	// An unresolved edition carries no denominator and must not be chosen, or
	// coverage becomes a division by an unknown.
	eds := []Edition{
		{ID: "unknown", TrackCount: 0},
		{ID: "known", TrackCount: 11},
	}
	got, ok := ChooseEdition(eds, 3)
	if !ok || got.ID != "known" {
		t.Errorf("want the edition with a known track count, got %+v", got)
	}
	if _, ok := ChooseEdition([]Edition{{ID: "x", TrackCount: 0}}, 3); ok {
		t.Error("no edition has a usable track count; the record cannot be judged")
	}
}

func TestChooseEdition_EmptyInput(t *testing.T) {
	if _, ok := ChooseEdition(nil, 5); ok {
		t.Error("no editions means no choice")
	}
}

func TestChooseEdition_IsDeterministic(t *testing.T) {
	// Equal track counts must not resolve by map order, or the same library
	// ranks differently between runs.
	eds := []Edition{
		{ID: "b", TrackCount: 11},
		{ID: "a", TrackCount: 11},
	}
	first, _ := ChooseEdition(eds, 5)
	for i := 0; i < 20; i++ {
		if got, _ := ChooseEdition(eds, 5); got.ID != first.ID {
			t.Fatalf("choice is not deterministic: %q then %q", first.ID, got.ID)
		}
	}
	if first.ID != "a" {
		t.Errorf("ties should break on id for stability, got %q", first.ID)
	}
}
