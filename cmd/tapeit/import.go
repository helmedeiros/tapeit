package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/apple"
	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/spotify"
)

// sourcePlaylist is one playlist read from a service. Err is a non-fatal
// per-playlist read failure: Tracks then holds whatever was read before it.
type sourcePlaylist struct {
	Name   string
	Tracks []importedTrack
	Err    error
}

// playlistSource is a service whose playlists can be read into the canonical
// lists. each visits every playlist so the runner can merge as they arrive.
type playlistSource interface {
	label() string
	each(ctx context.Context, fn func(sourcePlaylist) error) error
}

func cmdImport(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tapeit import (apple|spotify) [--out DIR]")
	}
	src, out, err := newSource(args[0], args[1:])
	if err != nil {
		return err
	}
	return runImport(ctx, src, out)
}

func newSource(name string, args []string) (playlistSource, string, error) {
	switch name {
	case "apple":
		fs := flag.NewFlagSet("import apple", flag.ContinueOnError)
		out := fs.String("out", "playlists", "directory holding the playlist JSON files")
		if err := fs.Parse(args); err != nil {
			return nil, "", err
		}
		creds, err := loadAppleCreds()
		if err != nil {
			return nil, "", fmt.Errorf("%w (run `tapeit auth apple` first)", err)
		}
		// Library reads carry the Music-User-Token, which only ValidateForWrite checks.
		if err := creds.ValidateForWrite(); err != nil {
			return nil, "", err
		}
		return appleSource{apple.NewClient(creds)}, *out, nil
	case "spotify":
		fs := flag.NewFlagSet("import spotify", flag.ContinueOnError)
		out := fs.String("out", "playlists", "directory holding the playlist JSON files")
		ownedOnly := fs.Bool("owned-only", false, "skip playlists you follow but don't own")
		if err := fs.Parse(args); err != nil {
			return nil, "", err
		}
		tok, err := loadToken()
		if err != nil {
			return nil, "", fmt.Errorf("%w (run `tapeit auth spotify` first)", err)
		}
		return spotifySource{client: spotify.NewClient(tok, saveToken), ownedOnly: *ownedOnly}, *out, nil
	default:
		return nil, "", fmt.Errorf("unknown import source %q (want apple|spotify)", name)
	}
}

func runImport(ctx context.Context, src playlistSource, out string) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	var written, partial int
	err := src.each(ctx, func(p sourcePlaylist) error {
		if p.Err != nil {
			fmt.Fprintf(os.Stderr, "  ! %s: incomplete (%d tracks read): %v\n", truncate(p.Name, 40), len(p.Tracks), p.Err)
			partial++
		}
		path := filepath.Join(out, slugify(p.Name)+".json")
		doc := mergeTracks(readPlaylistDoc(path, p.Name), p.Tracks)
		if err := writeJSON(path, doc); err != nil {
			return err
		}
		written++
		fmt.Printf("• %-45s %3d tracks → %s\n", truncate(p.Name, 45), len(doc.Tracks), path)
		return nil
	})
	if err != nil {
		return err
	}
	if written == 0 {
		fmt.Printf("no playlists found in %s\n", src.label())
		return nil
	}

	fmt.Printf("\n✓ imported %d playlist(s) from %s into %s/", written, src.label(), out)
	if partial > 0 {
		fmt.Printf(" — %d incomplete (re-run to retry)", partial)
	}
	fmt.Println()
	return nil
}

type appleSource struct{ client *apple.Client }

func (s appleSource) label() string { return "Apple Music" }

func (s appleSource) each(ctx context.Context, fn func(sourcePlaylist) error) error {
	playlists, err := s.client.ExistingPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("list playlists: %w", err)
	}
	for _, name := range sortedKeys(playlists) {
		tracks, err := s.client.PlaylistTracks(ctx, playlists[name])
		if e := fn(sourcePlaylist{Name: name, Tracks: fromAppleTracks(tracks), Err: err}); e != nil {
			return e
		}
	}
	return nil
}

type spotifySource struct {
	client    *spotify.Client
	ownedOnly bool
}

func (s spotifySource) label() string { return "Spotify" }

func (s spotifySource) each(ctx context.Context, fn func(sourcePlaylist) error) error {
	lib, err := s.client.Pull(ctx, spotify.PullOptions{
		OwnedOnly: s.ownedOnly,
		Progress:  func(m string) { fmt.Println(m) },
	})
	if err != nil {
		return err
	}
	for _, p := range lib.Playlists {
		if e := fn(sourcePlaylist{Name: p.Name, Tracks: fromSpotifyTracks(p.Tracks)}); e != nil {
			return e
		}
	}
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

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
