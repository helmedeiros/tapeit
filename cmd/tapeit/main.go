// Command tapeit migrates a music library from Spotify to Apple Music.
//
// See docs/DESIGN.md for architecture. This is the composition root: the only
// place where adapters are constructed and wired to the application use cases.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/helmedeiros/tapeit/internal/config"
	"github.com/helmedeiros/tapeit/internal/snapshot"
	"github.com/helmedeiros/tapeit/internal/spotify"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "tapeit:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "auth":
		return cmdAuth(ctx, args[1:])
	case "pull":
		return cmdPull(ctx, args[1:])
	case "version", "--version":
		fmt.Println("tapeit", version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage() {
	fmt.Print(`tapeit — migrate your music library from Spotify to Apple Music

Usage:
  tapeit auth spotify [--client-id ID]   Authorize with Spotify (PKCE)
  tapeit pull [--owned-only] [--out F]   Download your library into a snapshot
  tapeit version

Redirect URI to register in your Spotify app: ` + spotify.RedirectURI + `
`)
}

// --- auth ---

func cmdAuth(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "spotify" {
		return fmt.Errorf("usage: tapeit auth spotify [--client-id ID]")
	}
	fs := flag.NewFlagSet("auth spotify", flag.ContinueOnError)
	clientID := fs.String("client-id", os.Getenv("TAPEIT_SPOTIFY_CLIENT_ID"), "Spotify app Client ID")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *clientID == "" {
		return fmt.Errorf("missing Client ID: pass --client-id or set TAPEIT_SPOTIFY_CLIENT_ID")
	}

	tok, err := spotify.Login(ctx, *clientID)
	if err != nil {
		return err
	}
	if err := saveToken(tok); err != nil {
		return err
	}
	if err := saveApp(appSettings{SpotifyClientID: *clientID}); err != nil {
		return err
	}
	fmt.Println("✓ Authorized. Token saved. Run `tapeit pull` while Premium is active.")
	return nil
}

// --- pull ---

func cmdPull(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	ownedOnly := fs.Bool("owned-only", false, "skip playlists you follow but don't own")
	defaultOut, _ := config.SnapshotPath()
	out := fs.String("out", defaultOut, "snapshot output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tok, err := loadToken()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth spotify` first)", err)
	}

	client := spotify.NewClient(tok, saveToken)
	lib, err := client.Pull(ctx, spotify.PullOptions{
		OwnedOnly: *ownedOnly,
		Progress:  func(s string) { fmt.Println(s) },
	})
	if err != nil {
		return err
	}

	if err := snapshot.Save(*out, lib); err != nil {
		return err
	}
	fmt.Printf("\n✓ Pulled %d playlists, %d tracks → %s\n", len(lib.Playlists), lib.TrackCount(), *out)
	return nil
}

// --- local persistence (composition-root concern) ---

type appSettings struct {
	SpotifyClientID string `json:"spotify_client_id"`
}

func saveToken(t spotify.Token) error {
	path, err := config.TokenPath()
	if err != nil {
		return err
	}
	return writeJSON(path, t)
}

func loadToken() (spotify.Token, error) {
	var t spotify.Token
	path, err := config.TokenPath()
	if err != nil {
		return t, err
	}
	return t, readJSON(path, &t)
}

func saveApp(s appSettings) error {
	path, err := config.AppPath()
	if err != nil {
		return err
	}
	return writeJSON(path, s)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
