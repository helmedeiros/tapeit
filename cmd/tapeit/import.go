package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/spotify"
)

func cmdImport(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tapeit import (spotify) [--out DIR]")
	}
	switch args[0] {
	case "spotify":
		return cmdImportSpotify(ctx, args[1:])
	default:
		return fmt.Errorf("unknown import source %q (want spotify)", args[0])
	}
}

func cmdImportSpotify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("import spotify", flag.ContinueOnError)
	out := fs.String("out", "playlists", "directory holding the playlist JSON files")
	ownedOnly := fs.Bool("owned-only", false, "skip playlists you follow but don't own")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tok, err := loadToken()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth spotify` first)", err)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}

	client := spotify.NewClient(tok, saveToken)
	lib, err := client.Pull(ctx, spotify.PullOptions{
		OwnedOnly: *ownedOnly,
		Progress:  func(s string) { fmt.Println(s) },
	})
	if err != nil {
		return err
	}

	fmt.Println()
	for _, p := range lib.Playlists {
		path := filepath.Join(*out, slugify(p.Name)+".json")
		doc := mergeTracks(readPlaylistDoc(path, p.Name), fromSpotifyTracks(p.Tracks))
		if err := writeJSON(path, doc); err != nil {
			return err
		}
		fmt.Printf("• %-45s %3d tracks → %s\n", truncate(p.Name, 45), len(doc.Tracks), path)
	}

	fmt.Printf("\n✓ imported %d playlist(s) into %s/\n", len(lib.Playlists), *out)
	return nil
}

func fromSpotifyTracks(tracks []domain.Track) []importedTrack {
	out := make([]importedTrack, 0, len(tracks))
	for _, t := range tracks {
		out = append(out, importedTrack{
			Title:      t.Title,
			Artist:     strings.Join(t.Artists, ", "),
			Album:      t.Album,
			DurationMS: t.DurationMS,
			ISRC:       t.ISRC,
			Service:    "spotify",
			ServiceID:  t.SpotifyID,
		})
	}
	return out
}
