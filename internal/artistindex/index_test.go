package artistindex

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/helmedeiros/tapeit/internal/deezer"
)

type fakeSource struct{ calls int }

func (f *fakeSource) ArtistID(context.Context, string) (int64, bool, error) {
	f.calls++
	return 27, true, nil
}

func (f *fakeSource) RelatedArtists(context.Context, int64) ([]string, error) {
	f.calls++
	return []string{"Justice", "Cassius"}, nil
}

func (f *fakeSource) TopTracks(context.Context, int64, int) ([]deezer.Track, error) {
	f.calls++
	return []deezer.Track{{Title: "Genesis"}, {Title: "Stress"}}, nil
}

func TestRelatedCachesAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "idx.json")
	ctx := context.Background()

	ix, _ := Load(path)
	src := &fakeSource{}
	rel, err := ix.Related(ctx, src, "Daft Punk")
	if err != nil || len(rel) != 2 || rel[0] != "Justice" {
		t.Fatalf("related = %v, err %v", rel, err)
	}
	fetchCalls := src.calls
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	// Reload from disk; a second Related must not hit the source again.
	ix2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	src2 := &fakeSource{}
	rel2, err := ix2.Related(ctx, src2, "Daft Punk")
	if err != nil || len(rel2) != 2 {
		t.Fatalf("cached related = %v, err %v", rel2, err)
	}
	if src2.calls != 0 {
		t.Errorf("expected 0 source calls from cache, got %d", src2.calls)
	}
	if fetchCalls == 0 {
		t.Error("expected the first Related to hit the source")
	}
}

func TestTopTracksTagsArtist(t *testing.T) {
	ix, _ := Load(filepath.Join(t.TempDir(), "idx.json"))
	tops, err := ix.TopTracks(context.Background(), &fakeSource{}, "Justice", 2)
	if err != nil || len(tops) != 2 {
		t.Fatalf("top = %v, err %v", tops, err)
	}
	if tops[0].Artist != "Justice" {
		t.Errorf("want artist tagged Justice, got %q", tops[0].Artist)
	}
}
