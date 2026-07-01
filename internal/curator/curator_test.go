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
	got := m.Curate("Arctic Monkeys", Options{Size: 10})

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
	got := m.Curate("Arctic Monkeys", Options{Size: 10})
	for i := 1; i < len(got); i++ {
		if got[i].Artist == got[i-1].Artist {
			t.Errorf("adjacent same-artist at %d: %s", i, got[i].Artist)
		}
	}
}

func TestCurateBreadthLimitsNeighbours(t *testing.T) {
	m := Build(lib())
	got := m.Curate("Arctic Monkeys", Options{Size: 10, Breadth: 1})
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
	if got := m.Curate("Nonexistent Band", Options{Size: 10}); got != nil {
		t.Errorf("unknown seed should yield nil, got %v", got)
	}
}
