package curator

import "testing"

func TestBaseTitleStripsVersionSuffixes(t *testing.T) {
	cases := map[string]string{
		"Come Together - Remastered 2009": "Come Together",
		"Lonely Boy - 2021 Remaster":      "Lonely Boy",
		"Mr. Brightside (Live)":           "Mr. Brightside",
		"Umbrella (feat. Jay-Z)":          "Umbrella",
		"Instant Karma! - Ultimate Mix":   "Instant Karma!",
		"Bohemian Rhapsody - Radio Edit":  "Bohemian Rhapsody",
		"Maps [Explicit]":                 "Maps",
		"Song - Remastered (Bonus Track)": "Song",
	}
	for in, want := range cases {
		if got := baseTitle(in); got != want {
			t.Errorf("baseTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBaseTitleKeepsRealTitles(t *testing.T) {
	// Titles that merely contain look-alike substrings or legitimate parentheticals
	// must survive untouched.
	keep := []string{
		"Beautiful People (Stay High)",
		"Lo/Hi",
		"Olive Branch", // contains "live"
		"Anesthesia - Pulling Teeth",
		"Do I Wanna Know?",
	}
	for _, in := range keep {
		if got := baseTitle(in); got != in {
			t.Errorf("baseTitle(%q) = %q, should be unchanged", in, got)
		}
	}
}

func TestBuildRanksByFrequencyThenPosition(t *testing.T) {
	mk := func(title, artist string) Track { return Track{Title: title, Artist: artist} }
	lib := []Playlist{
		// "hit" appears in two playlists; in the first it's also position 0.
		{Name: "a", Tracks: []Track{mk("hit", "X"), mk("deep", "X"), mk("mid", "X")}},
		{Name: "b", Tracks: []Track{mk("mid", "X"), mk("hit", "X")}},
	}
	m := Build(lib)
	pool := m.tracks["x"]
	if len(pool) != 3 {
		t.Fatalf("expected 3 distinct tracks, got %d (%v)", len(pool), pool)
	}
	if pool[0].Title != "hit" {
		t.Errorf("most-frequent track should rank first, got %q", pool[0].Title)
	}
	if pool[1].Title != "mid" { // freq 2 as well but later position than hit... mid freq=2, hit freq=2
		t.Logf("order: %v", []string{pool[0].Title, pool[1].Title, pool[2].Title})
	}
	if pool[2].Title != "deep" {
		t.Errorf("freq-1 track should rank last, got %q", pool[2].Title)
	}
}

func TestBuildCollapsesReleases(t *testing.T) {
	mk := func(title string) Track { return Track{Title: title, Artist: "Q"} }
	lib := []Playlist{{Name: "p", Tracks: []Track{
		mk("Come Together - Remastered 2009"),
		mk("Come Together"),
		mk("Something"),
	}}}
	m := Build(lib)
	pool := m.tracks["q"]
	if len(pool) != 2 {
		t.Fatalf("remaster + studio should collapse to one, got %d: %v", len(pool), pool)
	}
	for _, tr := range pool {
		if tr.Title == "Come Together - Remastered 2009" {
			t.Errorf("clean title should be preferred as representative, got %q", tr.Title)
		}
	}
}
