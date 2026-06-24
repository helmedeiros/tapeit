// Package pusher recreates the source playlists in the target library. It is an
// application service depending only on domain.LibraryPort, so it is testable
// with a fake library and reusable across providers.
package pusher

import (
	"context"
	"fmt"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/matching"
)

// PlaylistState tracks per-playlist push progress for idempotent re-runs.
type PlaylistState struct {
	AppleID string `json:"apple_id"`
	Added   int    `json:"added"`
	Total   int    `json:"total"`
	Done    bool   `json:"done"`
}

// PushState is the full, persistable push progress keyed by playlist name.
type PushState struct {
	Playlists map[string]*PlaylistState `json:"playlists"`
}

// NewState returns an empty push state.
func NewState() *PushState { return &PushState{Playlists: map[string]*PlaylistState{}} }

func (s *PushState) get(name string) *PlaylistState {
	if s.Playlists == nil {
		s.Playlists = map[string]*PlaylistState{}
	}
	if s.Playlists[name] == nil {
		s.Playlists[name] = &PlaylistState{}
	}
	return s.Playlists[name]
}

// Service pushes playlists into a target library.
type Service struct {
	lib      domain.LibraryPort
	progress func(string)
}

// New builds a pusher. progress may be nil.
func New(lib domain.LibraryPort, progress func(string)) *Service {
	return &Service{lib: lib, progress: progress}
}

func (s *Service) report(format string, args ...any) {
	if s.progress != nil {
		s.progress(fmt.Sprintf(format, args...))
	}
}

// Push creates each playlist and adds its matched tracks. resolved maps a
// track Key (see matching.Key) to a catalog song id. state is updated and
// persisted via save after each playlist, so a re-run resumes safely.
func (s *Service) Push(ctx context.Context, playlists []domain.Playlist, resolved map[string]string, state *PushState, save func(*PushState) error) error {
	existing, err := s.lib.ExistingPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("list existing playlists: %w", err)
	}

	for _, p := range playlists {
		st := state.get(p.Name)
		if st.Done {
			s.report("skip %q (already done)", p.Name)
			continue
		}

		if st.AppleID == "" {
			if id, ok := existing[p.Name]; ok {
				st.AppleID = id
			} else {
				id, err := s.lib.CreatePlaylist(ctx, p.Name, p.Description)
				if err != nil {
					return fmt.Errorf("create %q: %w", p.Name, err)
				}
				st.AppleID = id
			}
			if err := save(state); err != nil {
				return err
			}
		}

		ids := resolveIDs(p.Tracks, resolved)
		st.Total = len(ids)
		if st.Added < len(ids) {
			if err := s.lib.AddTracks(ctx, st.AppleID, ids[st.Added:]); err != nil {
				return fmt.Errorf("add tracks to %q: %w", p.Name, err)
			}
			st.Added = len(ids)
		}
		st.Done = true
		if err := save(state); err != nil {
			return err
		}
		s.report("pushed %-40s %d/%d tracks", trunc(p.Name, 40), st.Added, len(p.Tracks))
	}
	return nil
}

// resolveIDs maps a playlist's tracks to catalog ids, preserving order and
// dropping unmatched tracks. Duplicate ids (same song twice) are kept once.
func resolveIDs(tracks []domain.Track, resolved map[string]string) []string {
	ids := make([]string, 0, len(tracks))
	seen := make(map[string]struct{}, len(tracks))
	for _, t := range tracks {
		id := resolved[matching.Key(t)]
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
