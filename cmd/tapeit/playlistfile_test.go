package main

import "testing"

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"This Is The Black Keys": "this-is-the-black-keys",
		"Último Romance":         "último-romance",
		"Harder, Better!":        "harder-better",
		"  spaced  out  ":        "spaced-out",
		"A.K.A. I-D-I-O-T":       "a-k-a-i-d-i-o-t",
		"???":                    "playlist",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergeTracksMatchesByBaseTitle(t *testing.T) {
	doc := playlistDoc{Name: "x", Tracks: []playlistTrack{{Title: "Lonely Boy", Artist: "The Black Keys"}}}
	doc = mergeTracks(doc, []importedTrack{
		{Title: "Lonely Boy - 2021 Remaster", Artist: "The Black Keys", Album: "El Camino", DurationMS: 193000, Service: "appleMusic", ServiceID: "42"},
	})
	if len(doc.Tracks) != 1 {
		t.Fatalf("want 1 track, got %d", len(doc.Tracks))
	}
	got := doc.Tracks[0]
	if got.Title != "Lonely Boy" || got.Album != "El Camino" || got.IDs["appleMusic"] != "42" {
		t.Errorf("merge did not enrich in place: %+v", got)
	}
}

func TestMergeTracksMatchesByISRCAcrossServices(t *testing.T) {
	doc := playlistDoc{Tracks: []playlistTrack{
		{Title: "Lonely Boy", Artist: "The Black Keys", ISRC: "USABC1234567", IDs: map[string]string{"appleMusic": "42"}},
	}}
	// Spotify reports a different title casing but the same ISRC.
	doc = mergeTracks(doc, []importedTrack{
		{Title: "LONELY BOY", Artist: "Black Keys", ISRC: "usabc1234567", Service: "spotify", ServiceID: "sp99"},
	})
	if len(doc.Tracks) != 1 {
		t.Fatalf("ISRC match should not append: got %d tracks", len(doc.Tracks))
	}
	ids := doc.Tracks[0].IDs
	if ids["appleMusic"] != "42" || ids["spotify"] != "sp99" {
		t.Errorf("want both service ids, got %v", ids)
	}
}

func TestMergeTracksAppendsUnknownAndKeepsIntended(t *testing.T) {
	doc := playlistDoc{Tracks: []playlistTrack{{Title: "Intended Only", Artist: "A"}}}
	doc = mergeTracks(doc, []importedTrack{{Title: "New Track", Artist: "B", Service: "spotify", ServiceID: "s1"}})
	if len(doc.Tracks) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(doc.Tracks))
	}
	if doc.Tracks[0].Title != "Intended Only" {
		t.Errorf("intended track dropped: %+v", doc.Tracks)
	}
}
