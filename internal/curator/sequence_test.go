package curator

import "testing"

func tb(title, artist string, bpm float64) Track {
	return Track{Title: title, Artist: artist, BPM: bpm}
}

func noAdjacentSameArtist(t *testing.T, got []Track) {
	t.Helper()
	for i := 1; i < len(got); i++ {
		if got[i].Artist == got[i-1].Artist {
			t.Errorf("adjacent same-artist at %d: %s", i, got[i].Artist)
		}
	}
}

func TestSequenceSmoothAscendsByBPM(t *testing.T) {
	in := []Track{
		tb("fast", "A", 160), tb("slow", "B", 80),
		tb("mid", "C", 120), tb("slowish", "D", 100),
	}
	got := Sequence(in, FlowSmooth)
	for i := 1; i < len(got); i++ {
		if got[i].BPM < got[i-1].BPM {
			t.Errorf("bpm should not decrease in smooth flow: %v -> %v", got[i-1].BPM, got[i].BPM)
		}
	}
	noAdjacentSameArtist(t, got)
}

func TestSequenceArcPeaksInMiddle(t *testing.T) {
	in := []Track{
		tb("t1", "A", 90), tb("t2", "B", 100), tb("t3", "C", 110),
		tb("t4", "D", 120), tb("t5", "E", 130),
	}
	got := Sequence(in, FlowArc)
	peak := 0
	for i, tr := range got {
		if tr.BPM > got[peak].BPM {
			peak = i
		}
	}
	if peak == 0 || peak == len(got)-1 {
		t.Errorf("arc peak should be interior, got index %d of %d", peak, len(got))
	}
	noAdjacentSameArtist(t, got)
}

func TestSequenceUnknownBPMGoLast(t *testing.T) {
	in := []Track{
		tb("has", "A", 120), tb("none1", "B", 0),
		tb("has2", "C", 100), tb("none2", "D", 0),
	}
	got := Sequence(in, FlowSmooth)
	if got[0].BPM == 0 || got[1].BPM == 0 {
		t.Errorf("bpm tracks should lead, got %v", got)
	}
	if got[len(got)-1].BPM != 0 {
		t.Errorf("unknown-bpm track should trail, got %v", got[len(got)-1])
	}
}

func TestSequenceKeepsArtistConstraintWhenBPMTies(t *testing.T) {
	// Same-artist tracks at the same tempo must still be separated.
	in := []Track{
		tb("a1", "A", 120), tb("a2", "A", 120),
		tb("b1", "B", 120), tb("b2", "B", 120),
	}
	noAdjacentSameArtist(t, Sequence(in, FlowSmooth))
}

func TestSequenceFallsBackWithoutBPM(t *testing.T) {
	in := []Track{tb("x", "A", 0), tb("y", "B", 0), tb("z", "A", 0)}
	got := Sequence(in, FlowSmooth)
	if len(got) != 3 {
		t.Fatalf("expected all tracks kept, got %d", len(got))
	}
	noAdjacentSameArtist(t, got)
}

func TestParseFlow(t *testing.T) {
	for in, want := range map[string]Flow{"": FlowNone, "none": FlowNone, "smooth": FlowSmooth, "ascending": FlowSmooth, "arc": FlowArc} {
		if got, ok := ParseFlow(in); !ok || got != want {
			t.Errorf("ParseFlow(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseFlow("boogie"); ok {
		t.Error("unknown flow should not parse")
	}
}
