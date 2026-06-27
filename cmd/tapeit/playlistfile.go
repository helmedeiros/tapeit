package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"

	"github.com/helmedeiros/tapeit/internal/matching"
)

type playlistDoc struct {
	Name   string          `json:"name"`
	Tracks []playlistTrack `json:"tracks"`
}

type playlistTrack struct {
	Title      string            `json:"title"`
	Artist     string            `json:"artist"`
	Album      string            `json:"album,omitempty"`
	DurationMS int               `json:"durationMs,omitempty"`
	ISRC       string            `json:"isrc,omitempty"`
	IDs        map[string]string `json:"ids,omitempty"`
}

// importedTrack is one track as a service reports it, ready to fold into a
// canonical list. Service/ServiceID record which catalog it came from.
type importedTrack struct {
	Title      string
	Artist     string
	Album      string
	DurationMS int
	ISRC       string
	Service    string
	ServiceID  string
}

// mergeTracks folds a service's tracks into the canonical list: a track already
// present (matched by ISRC, else by base title) gains the new album, duration,
// ISRC, and service id; a track the list lacks is appended; tracks the service
// does not carry are left untouched. The list stays service-agnostic — it only
// accrues metadata as it moves between services.
func mergeTracks(doc playlistDoc, tracks []importedTrack) playlistDoc {
	byISRC := make(map[string]int)
	byTitle := make(map[string]int)
	for i, t := range doc.Tracks {
		if t.ISRC != "" {
			byISRC[strings.ToUpper(t.ISRC)] = i
		}
		byTitle[trackKey(t.Title)] = i
	}

	for _, t := range tracks {
		i, ok := -1, false
		if t.ISRC != "" {
			i, ok = byISRC[strings.ToUpper(t.ISRC)]
		}
		if !ok {
			i, ok = byTitle[trackKey(t.Title)]
		}
		if !ok {
			doc.Tracks = append(doc.Tracks, playlistTrack{Title: t.Title, Artist: t.Artist})
			i = len(doc.Tracks) - 1
		}
		doc.Tracks[i] = enrich(doc.Tracks[i], t)
		if t.ISRC != "" {
			byISRC[strings.ToUpper(t.ISRC)] = i
		}
		byTitle[trackKey(doc.Tracks[i].Title)] = i
	}
	return doc
}

func enrich(t playlistTrack, src importedTrack) playlistTrack {
	if src.Album != "" {
		t.Album = src.Album
	}
	if src.DurationMS > 0 {
		t.DurationMS = src.DurationMS
	}
	if src.ISRC != "" {
		t.ISRC = src.ISRC
	}
	if src.Service != "" && src.ServiceID != "" {
		if t.IDs == nil {
			t.IDs = make(map[string]string)
		}
		t.IDs[src.Service] = src.ServiceID
	}
	return t
}

// trackKey identifies a recording by its base title (version and "(feat. …)"
// suffixes dropped) so a clean title and a service's fuller one resolve alike.
func trackKey(title string) string {
	if i := strings.Index(title, " - "); i > 0 {
		title = title[:i]
	}
	if i := strings.LastIndex(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		title = title[:i]
	}
	return matching.Normalize(title)
}

func readPlaylistDoc(path, name string) playlistDoc {
	f, err := os.Open(path)
	if err != nil {
		return playlistDoc{Name: name}
	}
	defer func() { _ = f.Close() }()
	doc := playlistDoc{Name: name}
	_ = json.NewDecoder(f).Decode(&doc)
	doc.Name = name
	return doc
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func slugify(name string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			prevHyphen = false
		default:
			if !prevHyphen && b.Len() > 0 {
				b.WriteRune('-')
			}
			prevHyphen = true
		}
	}
	if s := strings.Trim(b.String(), "-"); s != "" {
		return s
	}
	return "playlist"
}
