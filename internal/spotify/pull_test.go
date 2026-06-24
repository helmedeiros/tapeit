package spotify

import (
	"testing"
	"time"
)

func timeAhead(mins int) time.Time { return time.Now().Add(time.Duration(mins) * time.Minute) }
func timeAgo(mins int) time.Time   { return time.Now().Add(-time.Duration(mins) * time.Minute) }

func TestMapTrack(t *testing.T) {
	var d trackDTO
	d.ID = "abc"
	d.URI = "spotify:track:abc"
	d.Name = "Song"
	d.DurationMS = 210000
	d.Album.Name = "Album"
	d.Artists = []struct {
		Name string `json:"name"`
	}{{Name: "A1"}, {Name: "A2"}}
	d.ExternalIDs.ISRC = "USABC1234567"

	got := mapTrack(d)
	if got.Title != "Song" || got.Album != "Album" || got.ISRC != "USABC1234567" {
		t.Errorf("unexpected mapping: %+v", got)
	}
	if got.DurationMS != 210000 || got.SpotifyID != "abc" || got.SpotifyURI != "spotify:track:abc" {
		t.Errorf("unexpected mapping: %+v", got)
	}
	if len(got.Artists) != 2 || got.Artists[0] != "A1" || got.Artists[1] != "A2" {
		t.Errorf("artists not mapped: %+v", got.Artists)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("truncate short = %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("truncate = %q", got)
	}
}
