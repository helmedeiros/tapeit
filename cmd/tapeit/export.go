package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/helmedeiros/tapeit/internal/apple"
)

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
		doc := mergeTracks(readPlaylistDoc(path, name), fromAppleTracks(tracks))
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

func fromAppleTracks(tracks []apple.LibraryTrack) []importedTrack {
	out := make([]importedTrack, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, importedTrack{
			Title:      t.Title,
			Artist:     t.Artist,
			Album:      t.Album,
			DurationMS: t.DurationMS,
			Service:    "appleMusic",
			ServiceID:  t.CatalogID,
		})
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
