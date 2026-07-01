package main

import (
	"testing"

	"github.com/helmedeiros/tapeit/internal/deezer"
)

func TestPickDeezer(t *testing.T) {
	track := playlistTrack{Title: "Lonely Boy", Artist: "The Black Keys", DurationMS: 193000}

	cands := []deezer.Track{
		{ID: 1, Title: "Lonely Boy (Live)", Artist: "The Black Keys", DurationSec: 240},
		{ID: 2, Title: "Lonely Boy", Artist: "Some Cover Band", DurationSec: 193},
		{ID: 3, Title: "Lonely Boy", Artist: "The Black Keys", DurationSec: 193, ISRC: "USNO11100273"},
	}
	got, ok := pickDeezer(track, cands)
	if !ok || got.ID != 3 {
		t.Fatalf("want id 3 (exact title+artist+duration), got ok=%v %+v", ok, got)
	}
}

func TestPickDeezerRejectsFarDuration(t *testing.T) {
	track := playlistTrack{Title: "Song", Artist: "Artist", DurationMS: 180000}
	cands := []deezer.Track{{ID: 9, Title: "Song", Artist: "Artist", DurationSec: 60}} // 2min off
	if _, ok := pickDeezer(track, cands); ok {
		t.Error("expected rejection: duration too far off")
	}
}

func TestPickDeezerNoMatch(t *testing.T) {
	track := playlistTrack{Title: "Song", Artist: "Artist"}
	cands := []deezer.Track{{ID: 1, Title: "Different", Artist: "Artist"}}
	if _, ok := pickDeezer(track, cands); ok {
		t.Error("expected no match on title mismatch")
	}
}
