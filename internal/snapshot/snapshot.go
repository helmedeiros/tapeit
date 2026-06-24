// Package snapshot persists the pulled Spotify library to a local JSON file.
// This is the durable hand-off between the (Spotify-dependent) pull stage and
// the later match/push stages, so the source side only has to run once.
package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// Library is the full captured snapshot of a user's Spotify library.
type Library struct {
	PulledAt      time.Time         `json:"pulled_at"`
	SpotifyUserID string            `json:"spotify_user_id"`
	Playlists     []domain.Playlist `json:"playlists"`
}

// Save writes the library to path as indented JSON.
func Save(path string, lib Library) error {
	data, err := json.MarshalIndent(lib, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write snapshot %s: %w", path, err)
	}
	return nil
}

// Load reads a previously saved library snapshot.
func Load(path string) (Library, error) {
	var lib Library
	data, err := os.ReadFile(path)
	if err != nil {
		return lib, fmt.Errorf("read snapshot %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &lib); err != nil {
		return lib, fmt.Errorf("parse snapshot %s: %w", path, err)
	}
	return lib, nil
}

// TrackCount returns the total number of tracks across all playlists.
func (l Library) TrackCount() int {
	n := 0
	for _, p := range l.Playlists {
		n += len(p.Tracks)
	}
	return n
}
