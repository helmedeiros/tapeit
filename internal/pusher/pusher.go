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

// Push reconciles each source playlist into the target library. resolved maps
// a track Key (see matching.Key) to a catalog song id. For every playlist it
// finds or creates the target, reads what is already there, and adds only the
// missing tracks — so re-running never duplicates and fills in gaps. state is
// persisted via save after each playlist so an interrupted run resumes safely.
func (s *Service) Push(ctx context.Context, playlists []domain.Playlist, resolved map[string]string, state *PushState, save func(*PushState) error) error {
	existing, err := s.lib.ExistingPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("list existing playlists: %w", err)
	}

	for _, p := range playlists {
		st := state.get(p.Name)
		desired := resolveIDs(p.Tracks, resolved)
		st.Total = len(desired)

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

		current, err := s.lib.PlaylistTracks(ctx, st.AppleID)
		if err != nil {
			return fmt.Errorf("read %q: %w", p.Name, err)
		}

		toAdd, ordered := reconcile(current, desired)
		if !ordered {
			s.report("⚠ %q diverges from source order; appending %d missing track(s) at end", trunc(p.Name, 40), len(toAdd))
		}
		if len(toAdd) > 0 {
			if err := s.lib.AddTracks(ctx, st.AppleID, toAdd); err != nil {
				return fmt.Errorf("add tracks to %q: %w", p.Name, err)
			}
		}
		st.Added = len(desired)
		st.Done = true
		if err := save(state); err != nil {
			return err
		}
		s.report("✓ %-40s %d present +%d added (%d desired)", trunc(p.Name, 40), len(current), len(toAdd), len(desired))
	}
	return nil
}

// reconcile compares what is already in the playlist with the desired ordered
// ids and returns the ids to append. When current is a prefix of desired (the
// normal fresh-create and resume-after-interruption cases) the missing suffix
// is appended in order, so ordering is preserved. Otherwise the missing ids are
// still appended (to honor "add what's missing") but order is not guaranteed,
// and the caller is told via the returned ordered=false.
func reconcile(current, desired []string) (toAdd []string, ordered bool) {
	if isPrefix(current, desired) {
		return desired[len(current):], true
	}
	present := make(map[string]struct{}, len(current))
	for _, id := range current {
		present[id] = struct{}{}
	}
	for _, id := range desired {
		if _, ok := present[id]; !ok {
			toAdd = append(toAdd, id)
		}
	}
	return toAdd, false
}

func isPrefix(prefix, full []string) bool {
	if len(prefix) > len(full) {
		return false
	}
	for i, id := range prefix {
		if full[i] != id {
			return false
		}
	}
	return true
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
