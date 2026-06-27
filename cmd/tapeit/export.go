package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/helmedeiros/tapeit/internal/apple"
	"github.com/helmedeiros/tapeit/internal/matching"
)

type exportDoc struct {
	Name   string           `json:"name"`
	Tracks []exportDocTrack `json:"tracks"`
}

type exportDocTrack struct {
	Title      string            `json:"title"`
	Artist     string            `json:"artist"`
	Album      string            `json:"album,omitempty"`
	DurationMS int               `json:"durationMs,omitempty"`
	IDs        map[string]string `json:"ids,omitempty"`
}

func cmdExport(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("out", "playlists", "directory to write the playlist JSON files into")
	if err := fs.Parse(args); err != nil {
		return err
	}

	creds, err := loadAppleCreds()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth apple` first)", err)
	}
	// Library reads carry the Music-User-Token, which only ValidateForWrite checks.
	if err := creds.ValidateForWrite(); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	client := apple.NewClient(creds)
	playlists, err := client.ExistingPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("list playlists: %w", err)
	}
	if len(playlists) == 0 {
		fmt.Println("no playlists found in your Apple Music library")
		return nil
	}

	var written, partial int
	for _, name := range sortedKeys(playlists) {
		tracks, err := client.PlaylistTracks(ctx, playlists[name])
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: incomplete (%d tracks read): %v\n", truncate(name, 40), len(tracks), err)
			partial++
		}

		path := filepath.Join(*out, slugify(name)+".json")
		doc := enrichWithApple(readExportDoc(path, name), tracks)
		if err := writeJSON(path, doc); err != nil {
			return err
		}

		written++
		fmt.Printf("• %-45s %3d tracks → %s\n", truncate(name, 45), len(doc.Tracks), path)
	}

	fmt.Printf("\n✓ exported %d playlist(s) to %s/", written, *out)
	if partial > 0 {
		fmt.Printf(" — %d incomplete (re-run to retry)", partial)
	}
	fmt.Println()
	return nil
}

// enrichWithApple folds Apple's library tracks into the canonical list: a track
// already present (matched on its base title) gains Apple's album, duration, and
// catalog id; an Apple track the list lacks is appended; intended tracks Apple
// does not carry are left untouched.
func enrichWithApple(doc exportDoc, tracks []apple.LibraryTrack) exportDoc {
	at := make(map[string]int, len(doc.Tracks))
	for i, t := range doc.Tracks {
		at[trackKey(t.Title, t.Artist)] = i
	}
	for _, t := range tracks {
		key := trackKey(t.Title, t.Artist)
		i, ok := at[key]
		if !ok {
			at[key] = len(doc.Tracks)
			doc.Tracks = append(doc.Tracks, exportDocTrack{Title: t.Title, Artist: t.Artist})
			i = len(doc.Tracks) - 1
		}
		doc.Tracks[i] = withAppleMetadata(doc.Tracks[i], t)
	}
	return doc
}

func withAppleMetadata(t exportDocTrack, a apple.LibraryTrack) exportDocTrack {
	if a.Album != "" {
		t.Album = a.Album
	}
	if a.DurationMS > 0 {
		t.DurationMS = a.DurationMS
	}
	if a.CatalogID != "" {
		if t.IDs == nil {
			t.IDs = make(map[string]string)
		}
		t.IDs["appleMusic"] = a.CatalogID
	}
	return t
}

func readExportDoc(path, name string) exportDoc {
	f, err := os.Open(path)
	if err != nil {
		return exportDoc{Name: name}
	}
	defer func() { _ = f.Close() }()
	doc := exportDoc{Name: name}
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// trackKey identifies a recording within a playlist by its base title (version
// and "(feat. …)" suffixes dropped) so a clean intended title and Apple's fuller
// one resolve to the same track.
func trackKey(title, _ string) string {
	if i := strings.Index(title, " - "); i > 0 {
		title = title[:i]
	}
	if i := strings.LastIndex(title, " ("); i > 0 && strings.HasSuffix(title, ")") {
		title = title[:i]
	}
	return matching.Normalize(title)
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
