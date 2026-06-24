package pusher

import (
	"context"
	"testing"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/matching"
)

type fakeLibrary struct {
	existing map[string]string
	created  []string
	added    map[string][]string
	refs     map[string][]domain.TrackRef
	nextID   int
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{existing: map[string]string{}, added: map[string][]string{}, refs: map[string][]domain.TrackRef{}}
}

func (f *fakeLibrary) ExistingPlaylists(context.Context) (map[string]string, error) {
	return f.existing, nil
}

func (f *fakeLibrary) CreatePlaylist(_ context.Context, name, _ string) (string, error) {
	f.nextID++
	id := "pl-" + name
	f.created = append(f.created, name)
	return id, nil
}

// refs simulates the title+artist already present in a library playlist.
func (f *fakeLibrary) PlaylistTrackRefs(_ context.Context, playlistID string) ([]domain.TrackRef, error) {
	return f.refs[playlistID], nil
}

func (f *fakeLibrary) AddTracks(_ context.Context, playlistID string, songIDs []string) error {
	f.added[playlistID] = append(f.added[playlistID], songIDs...)
	return nil
}

func track(title, isrc string) domain.Track {
	return domain.Track{Title: title, ISRC: isrc}
}

func TestPush_CreatesAndAdds(t *testing.T) {
	lib := newFakeLibrary()
	playlists := []domain.Playlist{
		{Name: "Road Trip", Tracks: []domain.Track{track("A", "I1"), track("B", "I2"), track("C", "I3")}},
	}
	resolved := map[string]string{
		matching.Key(track("A", "I1")): "song-a",
		matching.Key(track("B", "I2")): "song-b",
		// C is unmatched on purpose
	}

	state := NewState()
	saves := 0
	save := func(*PushState) error { saves++; return nil }

	if err := New(lib, nil).Push(context.Background(), playlists, resolved, state, Options{}, save); err != nil {
		t.Fatal(err)
	}

	if len(lib.created) != 1 || lib.created[0] != "Road Trip" {
		t.Errorf("created = %v", lib.created)
	}
	got := lib.added["pl-Road Trip"]
	if len(got) != 2 || got[0] != "song-a" || got[1] != "song-b" {
		t.Errorf("added = %v, want [song-a song-b]", got)
	}
	if !state.Playlists["Road Trip"].Done {
		t.Errorf("playlist not marked done")
	}
	if saves == 0 {
		t.Errorf("expected state to be persisted")
	}
}

func TestReconcile(t *testing.T) {
	desired := []string{"a", "b", "c"}

	if add, ok := reconcile(nil, desired); !ok || len(add) != 3 {
		t.Errorf("empty current: add=%v ok=%v, want all in order", add, ok)
	}
	if add, ok := reconcile([]string{"a", "b"}, desired); !ok || len(add) != 1 || add[0] != "c" {
		t.Errorf("prefix: add=%v ok=%v, want [c] ordered", add, ok)
	}
	if add, ok := reconcile(desired, desired); !ok || len(add) != 0 {
		t.Errorf("equal: add=%v ok=%v, want none", add, ok)
	}
	// Divergent: "b" present but not a prefix -> still adds missing, flags order.
	add, ok := reconcile([]string{"b"}, desired)
	if ok {
		t.Errorf("divergent should report ordered=false")
	}
	if len(add) != 2 || add[0] != "a" || add[1] != "c" {
		t.Errorf("divergent add=%v, want [a c]", add)
	}
}

func TestPush_AddsMissingInOrder(t *testing.T) {
	lib := newFakeLibrary()
	playlists := []domain.Playlist{{Name: "Mix", Tracks: []domain.Track{
		track("A", "I1"), track("B", "I2"), track("C", "I3"),
	}}}
	resolved := map[string]string{
		matching.Key(track("A", "I1")): "song-a",
		matching.Key(track("B", "I2")): "song-b",
		matching.Key(track("C", "I3")): "song-c",
	}
	// A prior run created the playlist and recorded the first track as added.
	state := NewState()
	state.Playlists["Mix"] = &PlaylistState{AppleID: "pl-Mix", AddedIDs: []string{"song-a"}}

	if err := New(lib, nil).Push(context.Background(), playlists, resolved, state, Options{}, func(*PushState) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(lib.created) != 0 {
		t.Errorf("should reuse tracked playlist, not create: %v", lib.created)
	}
	// Only the newly-missing suffix is added, in order.
	if got := lib.added["pl-Mix"]; len(got) != 2 || got[0] != "song-b" || got[1] != "song-c" {
		t.Errorf("added = %v, want [song-b song-c]", got)
	}
	if got := state.Playlists["Mix"].AddedIDs; len(got) != 3 || got[0] != "song-a" || got[2] != "song-c" {
		t.Errorf("state AddedIDs = %v, want [song-a song-b song-c]", got)
	}
}

func TestPush_Idempotent(t *testing.T) {
	lib := newFakeLibrary()
	playlists := []domain.Playlist{{Name: "P", Tracks: []domain.Track{track("A", "I1")}}}
	resolved := map[string]string{matching.Key(track("A", "I1")): "song-a"}
	state := NewState()
	noop := func(*PushState) error { return nil }

	svc := New(lib, nil)
	if err := svc.Push(context.Background(), playlists, resolved, state, Options{}, noop); err != nil {
		t.Fatal(err)
	}
	// Second run with the same state must not create or add again.
	if err := svc.Push(context.Background(), playlists, resolved, state, Options{}, noop); err != nil {
		t.Fatal(err)
	}
	if len(lib.created) != 1 {
		t.Errorf("created %d times, want 1", len(lib.created))
	}
	if got := lib.added["pl-P"]; len(got) != 1 {
		t.Errorf("added %v, want one track only", got)
	}
}

func TestPush_SkipsUntrackedExisting(t *testing.T) {
	lib := newFakeLibrary()
	lib.existing["Manual"] = "pl-manual" // user-made, not in tapeit state
	playlists := []domain.Playlist{{Name: "Manual", Tracks: []domain.Track{track("A", "I1")}}}
	resolved := map[string]string{matching.Key(track("A", "I1")): "song-a"}
	noop := func(*PushState) error { return nil }

	// Default: leave the manual playlist untouched.
	if err := New(lib, nil).Push(context.Background(), playlists, resolved, NewState(), Options{}, noop); err != nil {
		t.Fatal(err)
	}
	if len(lib.added["pl-manual"]) != 0 || len(lib.created) != 0 {
		t.Errorf("should not touch untracked existing playlist: added=%v created=%v", lib.added["pl-manual"], lib.created)
	}

	// Adopt: fill it.
	if err := New(lib, nil).Push(context.Background(), playlists, resolved, NewState(), Options{Adopt: true}, noop); err != nil {
		t.Fatal(err)
	}
	if len(lib.added["pl-manual"]) != 1 {
		t.Errorf("adopt should add to existing playlist, got %v", lib.added["pl-manual"])
	}
}

func TestPush_AdoptDiffMerge(t *testing.T) {
	lib := newFakeLibrary()
	lib.existing["Mix"] = "pl-mix"
	// The user already added "Song A" by hand.
	lib.refs["pl-mix"] = []domain.TrackRef{{Title: "Song A", Artist: "Artist X"}}

	a := domain.Track{Title: "Song A", Artists: []string{"Artist X"}, ISRC: "I1"}
	b := domain.Track{Title: "Song B", Artists: []string{"Artist Y"}, ISRC: "I2"}
	playlists := []domain.Playlist{{Name: "Mix", Tracks: []domain.Track{a, b}}}
	resolved := map[string]string{
		matching.Key(a): "song-a",
		matching.Key(b): "song-b",
	}

	if err := New(lib, nil).Push(context.Background(), playlists, resolved, NewState(), Options{Adopt: true}, func(*PushState) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// Song A already present by title+artist -> only Song B is added.
	if got := lib.added["pl-mix"]; len(got) != 1 || got[0] != "song-b" {
		t.Errorf("adopt diff: added=%v, want [song-b] (A already present)", got)
	}
}
