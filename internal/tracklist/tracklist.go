// Package tracklist parses a plain-text song list into domain tracks, for
// creating a playlist from a list you supply by hand (e.g. a Spotify editorial
// playlist the API won't expose). One track per line:
//
//	Title - Artist
//
// " — " (em dash) and " | " are also accepted as separators; a line with no
// separator is treated as a title only. Blank lines and lines starting with
// "#" are ignored.
package tracklist

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// separators are tried in order; the first found splits title from artist.
var separators = []string{" — ", " | ", " - "}

// Parse reads a track list from r.
func Parse(r io.Reader) ([]domain.Track, error) {
	var tracks []domain.Track
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		title, artist := split(line)
		t := domain.Track{Title: title}
		if artist != "" {
			t.Artists = []string{artist}
		}
		tracks = append(tracks, t)
	}
	return tracks, sc.Err()
}

// ParseJSON reads a JSON playlist export of the form
//
//	{"name": "...", "tracks": [{"title": "...", "artist": "A, B"}]}
//
// returning the playlist name and its tracks. A comma-separated artist string
// becomes multiple artists (the first is treated as primary when matching).
func ParseJSON(r io.Reader) (name string, tracks []domain.Track, err error) {
	var doc struct {
		Name   string `json:"name"`
		Tracks []struct {
			Title  string `json:"title"`
			Artist string `json:"artist"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(r).Decode(&doc); err != nil {
		return "", nil, fmt.Errorf("parse json playlist: %w", err)
	}
	for _, t := range doc.Tracks {
		if strings.TrimSpace(t.Title) == "" {
			continue
		}
		tr := domain.Track{Title: strings.TrimSpace(t.Title)}
		for _, a := range strings.Split(t.Artist, ",") {
			if a = strings.TrimSpace(a); a != "" {
				tr.Artists = append(tr.Artists, a)
			}
		}
		tracks = append(tracks, tr)
	}
	return doc.Name, tracks, nil
}

func split(line string) (title, artist string) {
	for _, sep := range separators {
		if i := strings.Index(line, sep); i > 0 {
			return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i+len(sep):])
		}
	}
	return line, ""
}
