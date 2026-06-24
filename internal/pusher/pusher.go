// Package pusher recreates the source playlists in the target library. It is an
// application service depending only on domain.LibraryPort, so it is testable
// with a fake library and reusable across providers.
package pusher

import (
	"context"
	"fmt"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/matching"
)

// PlaylistState tracks per-playlist push progress for idempotent re-runs.
// AddedIDs is the ordered list of catalog song ids tapeIt has added — the
// source of truth for idempotency, since Apple does not reliably echo a
// track's catalog id back when reading a library playlist.
type PlaylistState struct {
	AppleID  string   `json:"apple_id"`
	AddedIDs []string `json:"added_ids"`
	Done     bool     `json:"done"`
	// Adopted marks a playlist that already existed (the user made it by hand).
	// Such playlists are reconciled by diffing their actual contents on title+
	// artist, since tapeit did not add the user's own tracks.
	Adopted bool `json:"adopted,omitempty"`
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

// Options tunes a push run.
type Options struct {
	// Adopt, when true, lets tapeIt write into a library playlist that already
	// exists by name but was not created by tapeIt (e.g. one you started
	// manually). Off by default to avoid duplicating tracks you added yourself.
	Adopt bool
}

// Push reconciles each source playlist into the target library. resolved maps a
// track Key (see matching.Key) to a catalog song id. tapeIt creates the
// playlist (or resumes one it created earlier) and adds only the tracks it has
// not already added, tracked in state — so re-running never duplicates and
// fills in gaps. state is persisted via save after each playlist so an
// interrupted run resumes safely.
func (s *Service) Push(ctx context.Context, playlists []domain.Playlist, resolved map[string]string, state *PushState, opts Options, save func(*PushState) error) error {
	existing, err := s.lib.ExistingPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("list existing playlists: %w", err)
	}

	for _, p := range playlists {
		st := state.get(p.Name)
		desired := resolveIDs(p.Tracks, resolved)

		if st.AppleID == "" {
			if id, ok := existing[p.Name]; ok {
				// Exists in the library but tapeIt has no record of it — likely
				// one you created manually. Skip unless explicitly adopting, to
				// avoid duplicating tracks you added yourself.
				if !opts.Adopt {
					s.report("• skip %-38s exists but not created by tapeit (use --adopt to fill it)", trunc(p.Name, 38))
					continue
				}
				st.AppleID = id
				st.Adopted = true
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

		var toAdd []string
		if st.Adopted {
			// Diff against the playlist's actual contents (by title+artist) and
			// add only the matched source tracks that aren't already there.
			refs, err := s.lib.PlaylistTrackRefs(ctx, st.AppleID)
			if err != nil {
				return fmt.Errorf("read %q: %w", p.Name, err)
			}
			toAdd = missingFromLibrary(p.Tracks, resolved, refs, st.AddedIDs)
		} else {
			var ordered bool
			toAdd, ordered = reconcile(st.AddedIDs, desired)
			if !ordered {
				s.report("⚠ %q diverges from recorded order; appending %d track(s) at end", trunc(p.Name, 40), len(toAdd))
			}
		}

		if len(toAdd) > 0 {
			if err := s.lib.AddTracks(ctx, st.AppleID, toAdd); err != nil {
				return fmt.Errorf("add tracks to %q: %w", p.Name, err)
			}
			st.AddedIDs = append(st.AddedIDs, toAdd...)
		}
		st.Done = true
		if err := save(state); err != nil {
			return err
		}
		verb := "had"
		if st.Adopted {
			verb = "adopt: kept"
		}
		s.report("✓ %-40s %s, +%d added (%d desired)", trunc(p.Name, 40), verb, len(toAdd), len(desired))
	}
	return nil
}

// missingFromLibrary returns the catalog ids of source tracks that are matched
// but not already in the playlist, preserving source order and dropping
// duplicates. A track counts as present if either tapeIt already added its
// catalog id (alreadyAdded — reliable, keeps re-runs idempotent even when
// Apple renders a track's title differently than the source) or its normalized
// title+artist is in the library (present — covers tracks the user added by
// hand, which have no recorded id).
func missingFromLibrary(tracks []domain.Track, resolved map[string]string, present []domain.TrackRef, alreadyAdded []string) []string {
	have := make(map[string]struct{}, len(present))
	for _, r := range present {
		have[refKey(r.Title, r.Artist)] = struct{}{}
	}
	addedSet := make(map[string]struct{}, len(alreadyAdded))
	for _, id := range alreadyAdded {
		addedSet[id] = struct{}{}
	}
	var ids []string
	seen := make(map[string]struct{})
	for _, t := range tracks {
		id := resolved[matching.Key(t)]
		if id == "" {
			continue
		}
		if _, ok := addedSet[id]; ok {
			continue // tapeIt already added this one
		}
		artist := ""
		if len(t.Artists) > 0 {
			artist = strings.Join(t.Artists, " ")
		}
		if _, ok := have[refKey(t.Title, artist)]; ok {
			continue // already in the playlist (e.g. added by hand)
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func refKey(title, artist string) string {
	return matching.Normalize(title) + "|" + matching.Normalize(artist)
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
