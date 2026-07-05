package curator

import "testing"

func lib() []Playlist {
	t := func(title, artist string) Track { return Track{Title: title, Artist: artist} }
	return []Playlist{
		// Arctic Monkeys grouped with Strokes and Franz Ferdinand repeatedly.
		{Name: "indie 1", Tracks: []Track{t("505", "Arctic Monkeys"), t("Last Nite", "The Strokes"), t("Take Me Out", "Franz Ferdinand")}},
		{Name: "indie 2", Tracks: []Track{t("R U Mine", "Arctic Monkeys"), t("Reptilia", "The Strokes"), t("Do You Want To", "Franz Ferdinand")}},
		// A separate cluster that never co-occurs with Arctic Monkeys.
		{Name: "jazz", Tracks: []Track{t("So What", "Miles Davis"), t("Blue Train", "John Coltrane")}},
	}
}

func TestCurateExpandsFromSeedByCooccurrence(t *testing.T) {
	m := Build(lib())
	got := m.Curate([]string{"Arctic Monkeys"}, Options{Size: 10})

	artists := map[string]bool{}
	for _, tr := range got {
		artists[tr.Artist] = true
	}
	if !artists["Arctic Monkeys"] {
		t.Error("seed artist missing from result")
	}
	if !artists["The Strokes"] || !artists["Franz Ferdinand"] {
		t.Errorf("expected co-occurring artists pulled in, got %v", artists)
	}
	if artists["Miles Davis"] || artists["John Coltrane"] {
		t.Errorf("unrelated cluster should not appear, got %v", artists)
	}
}

func TestCurateSeparatesArtists(t *testing.T) {
	m := Build(lib())
	got := m.Curate([]string{"Arctic Monkeys"}, Options{Size: 10})
	for i := 1; i < len(got); i++ {
		if got[i].Artist == got[i-1].Artist {
			t.Errorf("adjacent same-artist at %d: %s", i, got[i].Artist)
		}
	}
}

func TestCurateBreadthLimitsNeighbours(t *testing.T) {
	m := Build(lib())
	got := m.Curate([]string{"Arctic Monkeys"}, Options{Size: 10, Breadth: 1})
	artists := map[string]bool{}
	for _, tr := range got {
		artists[tr.Artist] = true
	}
	if len(artists) > 2 { // seed + at most one neighbour
		t.Errorf("breadth 1 should give seed + 1 neighbour, got %v", artists)
	}
}

func TestCurateUnknownSeed(t *testing.T) {
	m := Build(lib())
	if got := m.Curate([]string{"Nonexistent Band"}, Options{Size: 10}); got != nil {
		t.Errorf("unknown seed should yield nil, got %v", got)
	}
}

func TestEvaluateFocusBeatsPopularity(t *testing.T) {
	// A library where co-occurrence is informative: two tight genre clusters that
	// never mix. Held-out cluster-mates should be recoverable from the seed by
	// co-occurrence, but not by raw popularity.
	mk := func(title, artist string) Track { return Track{Title: title, Artist: artist} }
	rock := []Track{
		mk("a", "Arctic Monkeys"), mk("b", "The Strokes"), mk("c", "Franz Ferdinand"),
		mk("d", "The Killers"), mk("e", "Interpol"), mk("f", "The Kooks"),
		mk("g", "Kaiser Chiefs"), mk("h", "Editors"),
	}
	jazz := []Track{
		mk("i", "Miles Davis"), mk("j", "John Coltrane"), mk("k", "Bill Evans"),
		mk("l", "Charles Mingus"), mk("m", "Thelonious Monk"), mk("n", "Herbie Hancock"),
		mk("o", "Wayne Shorter"), mk("p", "Chet Baker"),
	}
	lib := []Playlist{
		{Name: "rock 1", Tracks: rock},
		{Name: "rock 2", Tracks: rock},
		{Name: "jazz 1", Tracks: jazz},
		{Name: "jazz 2", Tracks: jazz},
	}
	r := Evaluate(lib, EvalOptions{MinArtists: 8, Holdout: 0.4, K: 3})
	if r.Playlists == 0 {
		t.Fatal("expected some playlists evaluated")
	}
	if r.Recall <= r.BaselineRecall {
		t.Errorf("focus recall %.3f should beat popularity %.3f", r.Recall, r.BaselineRecall)
	}
}

func TestCurateMultipleSeeds(t *testing.T) {
	m := Build(lib())
	// Seeding both clusters should surface tracks from both, and never place the
	// seeds themselves as neighbours of each other.
	got := m.Curate([]string{"Arctic Monkeys", "Miles Davis"}, Options{Size: 20})
	artists := map[string]bool{}
	for _, tr := range got {
		artists[tr.Artist] = true
	}
	if !artists["Arctic Monkeys"] || !artists["Miles Davis"] {
		t.Errorf("both seeds should appear, got %v", artists)
	}
	if !artists["The Strokes"] || !artists["John Coltrane"] {
		t.Errorf("neighbours of each seed should appear, got %v", artists)
	}
	// One unknown seed among known ones is simply ignored.
	if got := m.Curate([]string{"Nobody", "Arctic Monkeys"}, Options{Size: 6}); len(got) == 0 {
		t.Error("a known seed alongside an unknown one should still curate")
	}
}
