// Package config resolves the on-disk locations tapeIt uses for tokens, app
// settings, and the library snapshot. Everything lives outside the repo so that
// personal data and credentials are never committed.
package config

import (
	"os"
	"path/filepath"
)

// Dir returns the tapeIt config directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "tapeit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// TokenPath is where the Spotify OAuth token is persisted.
func TokenPath() (string, error) { return inDir("spotify_token.json") }

// AppPath is where non-secret app settings (e.g. client id) are persisted.
func AppPath() (string, error) { return inDir("app.json") }

// SnapshotPath is the default path for the pulled library snapshot.
func SnapshotPath() (string, error) { return inDir("snapshot.json") }

// AppleCredsPath is where the extracted Apple Music tokens are persisted.
func AppleCredsPath() (string, error) { return inDir("apple_creds.json") }

// MatchesPath is where catalog match results are persisted.
func MatchesPath() (string, error) { return inDir("matches.json") }

// PushStatePath tracks push progress for idempotent re-runs.
func PushStatePath() (string, error) { return inDir("push_state.json") }

func inDir(name string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}
