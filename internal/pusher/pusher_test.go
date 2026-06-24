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
	nextID   int
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{existing: map[string]string{}, added: map[string][]string{}}
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

	if err := New(lib, nil).Push(context.Background(), playlists, resolved, state, save); err != nil {
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

func TestPush_Idempotent(t *testing.T) {
	lib := newFakeLibrary()
	playlists := []domain.Playlist{{Name: "P", Tracks: []domain.Track{track("A", "I1")}}}
	resolved := map[string]string{matching.Key(track("A", "I1")): "song-a"}
	state := NewState()
	noop := func(*PushState) error { return nil }

	svc := New(lib, nil)
	if err := svc.Push(context.Background(), playlists, resolved, state, noop); err != nil {
		t.Fatal(err)
	}
	// Second run with the same state must not create or add again.
	if err := svc.Push(context.Background(), playlists, resolved, state, noop); err != nil {
		t.Fatal(err)
	}
	if len(lib.created) != 1 {
		t.Errorf("created %d times, want 1", len(lib.created))
	}
	if got := lib.added["pl-P"]; len(got) != 1 {
		t.Errorf("added %v, want one track only", got)
	}
}
