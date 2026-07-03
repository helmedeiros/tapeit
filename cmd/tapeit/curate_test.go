package main

import (
	"path/filepath"
	"testing"
)

func TestResolvePlaylist(t *testing.T) {
	cases := []struct {
		v, dir, want string
	}{
		{"indie-rock-club", "playlists", filepath.Join("playlists", "indie-rock-club.json")},
		{"Indie Rock Club", "playlists", filepath.Join("playlists", "indie-rock-club.json")},
		{"/tmp/mix.json", "playlists", "/tmp/mix.json"},
		{"sub/mix.json", "playlists", "sub/mix.json"},
	}
	for _, c := range cases {
		if got := resolvePlaylist(c.v, c.dir); got != c.want {
			t.Errorf("resolvePlaylist(%q,%q)=%q want %q", c.v, c.dir, got, c.want)
		}
	}
}

func TestAssembleSeedsExplicitOnly(t *testing.T) {
	seeds, name, exclude, err := assembleSeeds([]string{"Nirvana", "Pixies"}, "", "playlists")
	if err != nil {
		t.Fatal(err)
	}
	if name != "" || exclude != nil {
		t.Errorf("no seed-playlist should give empty name and nil exclude, got %q %v", name, exclude)
	}
	if len(seeds) != 2 || seeds[0] != "Nirvana" {
		t.Errorf("seeds passed through, got %v", seeds)
	}
}

func TestAssembleSeedsFromPlaylist(t *testing.T) {
	dir := t.TempDir()
	doc := playlistDoc{Name: "Indie Rock Club", Tracks: []playlistTrack{
		{Title: "505", Artist: "Arctic Monkeys"},
		{Title: "Last Nite", Artist: "The Strokes"},
	}}
	if err := writeJSON(filepath.Join(dir, "indie-rock-club.json"), doc); err != nil {
		t.Fatal(err)
	}

	seeds, name, exclude, err := assembleSeeds([]string{"Pixies"}, "indie-rock-club", dir)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Indie Rock Club" {
		t.Errorf("source name = %q", name)
	}
	if len(seeds) != 3 { // explicit Pixies + two playlist artists
		t.Errorf("seeds = %v", seeds)
	}
	if !exclude["505|arctic monkeys"] {
		t.Errorf("source tracks should be excluded, got %v", exclude)
	}
}

func TestAssembleSeedsMissingPlaylist(t *testing.T) {
	if _, _, _, err := assembleSeeds(nil, "nope", t.TempDir()); err == nil {
		t.Error("missing --seed-playlist should error")
	}
}
