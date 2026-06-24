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
	"strings"

	"github.com/helmedeiros/tapeit/internal/apple"
	"github.com/helmedeiros/tapeit/internal/config"
	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/matching"
	"github.com/helmedeiros/tapeit/internal/pusher"
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
	case "match":
		return cmdMatch(ctx, args[1:])
	case "push":
		return cmdPush(ctx, args[1:])
	case "report":
		return cmdReport(args[1:])
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
  tapeit auth apple   [--dev-token T --user-token U --storefront S]
  tapeit pull   [--owned-only] [--out F]  Download your library into a snapshot
  tapeit match  [--out F]                 Resolve tracks to Apple catalog ids
  tapeit report                           Show match summary
  tapeit push   [--dry-run]               Create playlists in Apple Music
  tapeit version

Spotify redirect URI to register: ` + spotify.RedirectURI + `
`)
}

// --- auth ---

func cmdAuth(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: tapeit auth (spotify|apple) …")
	}
	switch args[0] {
	case "spotify":
		return cmdAuthSpotify(ctx, args[1:])
	case "apple":
		return cmdAuthApple(ctx, args[1:])
	default:
		return fmt.Errorf("unknown auth provider %q (want spotify|apple)", args[0])
	}
}

func cmdAuthSpotify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("auth spotify", flag.ContinueOnError)
	clientID := fs.String("client-id", os.Getenv("TAPEIT_SPOTIFY_CLIENT_ID"), "Spotify app Client ID")
	if err := fs.Parse(args); err != nil {
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

func cmdAuthApple(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("auth apple", flag.ContinueOnError)
	dev := fs.String("dev-token", os.Getenv("TAPEIT_APPLE_DEV_TOKEN"), "Apple Music developer token (from the web player)")
	user := fs.String("user-token", os.Getenv("TAPEIT_APPLE_USER_TOKEN"), "media-user-token cookie value")
	store := fs.String("storefront", os.Getenv("TAPEIT_APPLE_STOREFRONT"), "storefront id, e.g. de (auto-detected if omitted)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	// Merge over any previously saved creds, so the developer token (needed for
	// `match`) and the user token (needed only for `push`) can be supplied in
	// separate runs without re-pasting the other.
	creds, _ := loadAppleCreds()
	if *dev != "" {
		creds.DeveloperToken = *dev
	}
	if *user != "" {
		creds.UserToken = *user
	}
	if *store != "" {
		creds.Storefront = *store
	}

	if creds.DeveloperToken == "" {
		fmt.Print(appleInstructions)
		return fmt.Errorf("missing developer token: pass --dev-token")
	}
	if creds.Storefront == "" {
		if creds.UserToken == "" {
			return fmt.Errorf("set the storefront: pass --storefront <cc> (e.g. de), or --user-token to auto-detect")
		}
		sf, err := apple.NewClient(creds).Storefront(ctx)
		if err != nil {
			return fmt.Errorf("auto-detect storefront failed (pass --storefront): %w", err)
		}
		creds.Storefront = sf
		fmt.Println("Detected storefront:", sf)
	}
	if err := saveAppleCreds(creds); err != nil {
		return err
	}

	if creds.UserToken == "" {
		fmt.Println("✓ Saved developer token + storefront. Run `tapeit match`.")
		fmt.Println("  (Add --user-token before `tapeit push` — the write step.)")
	} else {
		fmt.Println("✓ Apple credentials saved. Run `tapeit match`, then `tapeit push`.")
	}
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

// --- match ---

func cmdMatch(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("match", flag.ContinueOnError)
	defaultOut, _ := config.MatchesPath()
	out := fs.String("out", defaultOut, "matches output path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lib, err := loadSnapshot()
	if err != nil {
		return err
	}
	creds, err := loadAppleCreds()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth apple` first)", err)
	}
	if err := creds.Validate(); err != nil {
		return err
	}

	unique := uniqueTracks(lib)

	// Resume: keep already-resolved tracks from a prior run, only (re)match the
	// rest. Unmatched entries are retried.
	prior := map[string]domain.Match{}
	if pm, err := loadMatches(); err == nil {
		for _, m := range pm.Matches {
			prior[matching.Key(m.Track)] = m
		}
	}
	var todo []domain.Track
	for _, t := range unique {
		if m, ok := prior[matching.Key(t)]; ok && m.Matched() {
			continue
		}
		todo = append(todo, t)
	}
	fmt.Printf("matching %d tracks (%d already resolved, %d total unique)…\n", len(todo), len(unique)-len(todo), len(unique))

	svc := matching.New(apple.NewClient(creds), func(s string) { fmt.Println(s) })
	fresh, err := svc.Match(ctx, todo)
	if err != nil {
		return err
	}
	for _, m := range fresh {
		prior[matching.Key(m.Track)] = m
	}

	// Emit in stable unique-track order.
	matches := make([]domain.Match, 0, len(unique))
	for _, t := range unique {
		matches = append(matches, prior[matching.Key(t)])
	}
	if err := snapshot.SaveMatches(*out, snapshot.Matches{Matches: matches}); err != nil {
		return err
	}

	summarize(matches)
	fmt.Printf("\n✓ Saved matches → %s\nReview with `tapeit report`, then `tapeit push`.\n", *out)
	return nil
}

// --- report ---

func cmdReport(_ []string) error {
	m, err := loadMatches()
	if err != nil {
		return err
	}
	summarize(m.Matches)
	var unmatched []domain.Match
	for _, x := range m.Matches {
		if !x.Matched() {
			unmatched = append(unmatched, x)
		}
	}
	if len(unmatched) > 0 {
		fmt.Printf("\nUnmatched (%d):\n", len(unmatched))
		for i, x := range unmatched {
			if i >= 50 {
				fmt.Printf("  … and %d more (see matches.json)\n", len(unmatched)-50)
				break
			}
			fmt.Printf("  - %s — %s\n", x.Track.Title, joinArtists(x.Track.Artists))
		}
	}
	return nil
}

// --- push ---

func cmdPush(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "report what would be pushed without writing")
	completeOnly := fs.Bool("complete-only", false, "push only playlists whose tracks are all matched")
	maxMissing := fs.Int("max-missing", -1, "push only playlists missing at most N tracks (-1 = no limit)")
	adopt := fs.Bool("adopt", false, "also fill playlists that already exist but tapeit didn't create (risks duplicating manual tracks)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	lib, err := loadSnapshot()
	if err != nil {
		return err
	}
	// Assign stable, unique display names (over the full set, so they don't
	// shift between runs) — handles empty and duplicate Spotify playlist names.
	playlists := uniqueNames(lib.Playlists)

	m, err := loadMatches()
	if err != nil {
		return err
	}
	resolved := resolvedIndex(m.Matches)

	limit := *maxMissing
	if *completeOnly {
		limit = 0
	}
	if limit >= 0 {
		all := playlists
		playlists = withinMissing(all, resolved, limit)
		fmt.Printf("filter: %d of %d playlists missing ≤%d tracks\n", len(playlists), len(all), limit)
	}

	if *dryRun {
		fmt.Printf("dry-run: would push %d playlists (%d tracks resolved)\n", len(playlists), countResolved(playlists, resolved))
		for _, p := range playlists {
			fmt.Printf("  %-44s %d tracks\n", truncate(p.Name, 44), len(p.Tracks))
		}
		return nil
	}

	creds, err := loadAppleCreds()
	if err != nil {
		return fmt.Errorf("%w (run `tapeit auth apple` first)", err)
	}
	if err := creds.ValidateForWrite(); err != nil {
		return err
	}

	state, err := loadPushState()
	if err != nil {
		return err
	}
	svc := pusher.New(apple.NewClient(creds), func(s string) { fmt.Println(s) })
	if err := svc.Push(ctx, playlists, resolved, state, pusher.Options{Adopt: *adopt}, savePushState); err != nil {
		return err
	}
	fmt.Println("\n✓ Push complete.")
	return nil
}

// --- shared helpers ---

func uniqueTracks(lib snapshot.Library) []domain.Track {
	seen := make(map[string]struct{})
	var out []domain.Track
	for _, p := range lib.Playlists {
		for _, t := range p.Tracks {
			k := matching.Key(t)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// uniqueNames assigns stable, unique display names so empty or duplicate
// Spotify playlist names don't collide (which would merge them on Apple).
func uniqueNames(pls []domain.Playlist) []domain.Playlist {
	seen := map[string]int{}
	out := make([]domain.Playlist, len(pls))
	for i, p := range pls {
		base := strings.TrimSpace(p.Name)
		if base == "" {
			base = "Untitled Playlist"
		}
		seen[base]++
		if seen[base] > 1 {
			p.Name = fmt.Sprintf("%s (%d)", base, seen[base])
		} else {
			p.Name = base
		}
		out[i] = p
	}
	return out
}

// withinMissing returns playlists with at most limit unmatched tracks.
func withinMissing(pls []domain.Playlist, resolved map[string]string, limit int) []domain.Playlist {
	var out []domain.Playlist
	for _, p := range pls {
		missing := 0
		for _, t := range p.Tracks {
			if resolved[matching.Key(t)] == "" {
				missing++
			}
		}
		if missing <= limit {
			out = append(out, p)
		}
	}
	return out
}

func countResolved(pls []domain.Playlist, resolved map[string]string) int {
	n := 0
	for _, p := range pls {
		for _, t := range p.Tracks {
			if resolved[matching.Key(t)] != "" {
				n++
			}
		}
	}
	return n
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func resolvedIndex(matches []domain.Match) map[string]string {
	idx := make(map[string]string)
	for _, m := range matches {
		if m.Matched() {
			idx[matching.Key(m.Track)] = m.AppleID
		}
	}
	return idx
}

func summarize(matches []domain.Match) {
	byConf := map[domain.Confidence]int{}
	for _, m := range matches {
		byConf[m.Confidence]++
	}
	total := len(matches)
	matched := byConf[domain.ConfExact] + byConf[domain.ConfHigh] + byConf[domain.ConfLow]
	fmt.Printf("\nMatch summary (%d unique tracks):\n", total)
	fmt.Printf("  exact (ISRC): %d\n", byConf[domain.ConfExact])
	fmt.Printf("  high:         %d\n", byConf[domain.ConfHigh])
	fmt.Printf("  low:          %d\n", byConf[domain.ConfLow])
	fmt.Printf("  unmatched:    %d\n", byConf[domain.ConfNone])
	if total > 0 {
		fmt.Printf("  → %d matched (%.1f%%)\n", matched, 100*float64(matched)/float64(total))
	}
}

func joinArtists(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}

const appleInstructions = `To get your Apple Music tokens (no paid account needed):

  1. Open https://music.apple.com and log in.
  2. Open DevTools (Cmd+Opt+I) → Network tab. Filter: amp-api
  3. Click any playlist so requests fire. Click an "amp-api.music.apple.com"
     request → Headers → Request Headers and copy:
       - Authorization: Bearer <DEVELOPER TOKEN>   (the long value after Bearer)
       - media-user-token: <USER TOKEN>
     (You can also read media-user-token from Application → Cookies.)
  4. Run:
       tapeit auth apple --dev-token "<DEVELOPER TOKEN>" --user-token "<USER TOKEN>"

`

// --- local persistence (composition-root concern) ---

type appSettings struct {
	SpotifyClientID string `json:"spotify_client_id"`
}

func saveToken(t spotify.Token) error          { return saveAt(config.TokenPath, t) }
func saveApp(s appSettings) error              { return saveAt(config.AppPath, s) }
func saveAppleCreds(c apple.Credentials) error { return saveAt(config.AppleCredsPath, c) }
func savePushState(s *pusher.PushState) error  { return saveAt(config.PushStatePath, s) }

func loadToken() (spotify.Token, error) {
	var t spotify.Token
	return t, loadAt(config.TokenPath, &t)
}

func loadAppleCreds() (apple.Credentials, error) {
	var c apple.Credentials
	return c, loadAt(config.AppleCredsPath, &c)
}

func loadSnapshot() (snapshot.Library, error) {
	path, err := config.SnapshotPath()
	if err != nil {
		return snapshot.Library{}, err
	}
	return snapshot.Load(path)
}

func loadMatches() (snapshot.Matches, error) {
	path, err := config.MatchesPath()
	if err != nil {
		return snapshot.Matches{}, err
	}
	return snapshot.LoadMatches(path)
}

func loadPushState() (*pusher.PushState, error) {
	path, err := config.PushStatePath()
	if err != nil {
		return nil, err
	}
	state := pusher.NewState()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return state, nil
	}
	if err := loadAt(config.PushStatePath, state); err != nil {
		return nil, err
	}
	return state, nil
}

func saveAt(pathFn func() (string, error), v any) error {
	path, err := pathFn()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadAt(pathFn func() (string, error), v any) error {
	path, err := pathFn()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
