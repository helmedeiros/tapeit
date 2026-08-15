package musicapp

import (
	"context"
	"errors"
	"testing"
)

// readerReturning builds a Reader over a canned script result, so parsing is
// tested without a Music app or a macOS host.
func readerReturning(out string, err error) *Reader {
	return &Reader{run: func(context.Context) (string, error) { return out, err }}
}

func TestPlayCounts_ReadsEveryField(t *testing.T) {
	r := readerReturning("6\tTake Me Out\tFranz Ferdinand\tFranz Ferdinand\n"+
		"3\tJacqueline\tFranz Ferdinand\tFranz Ferdinand\n", nil)

	got, err := r.PlayCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("read %d tracks, want 2", len(got))
	}
	if got[0].Count != 6 || got[0].Title != "Take Me Out" ||
		got[0].Artist != "Franz Ferdinand" || got[0].Album != "Franz Ferdinand" {
		t.Errorf("first row mapped wrong: %+v", got[0])
	}
}

func TestPlayCounts_KeepsNamesContainingPunctuation(t *testing.T) {
	// Track and album names carry commas, dashes, quotes and non-Latin script.
	// Only a tab cannot occur inside one, which is why it separates the fields.
	r := readerReturning("2\tYou Know I'm No Good\tAmy Winehouse\tBack to Black\n"+
		"4\tSprawl II (Mountains Beyond Mountains)\tArcade Fire\tThe Suburbs\n"+
		"1\tSuis-moi\tCamille & Hans Zimmer\tLe Petit Prince (Bande originale du film)\n", nil)

	got, err := r.PlayCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d tracks, want 3", len(got))
	}
	if got[1].Title != "Sprawl II (Mountains Beyond Mountains)" {
		t.Errorf("parenthesised title lost: %q", got[1].Title)
	}
	if got[2].Album != "Le Petit Prince (Bande originale du film)" {
		t.Errorf("accented album lost: %q", got[2].Album)
	}
}

func TestPlayCounts_DropsRowsItCannotRead(t *testing.T) {
	// A malformed row costs one track. Failing the whole read would cost the
	// entire signal because one album title contained something unexpected,
	// which is the worse trade for evidence that only ever improves a ranking.
	r := readerReturning("6\tGood\tArtist\tAlbum\n"+
		"notanumber\tBad count\tArtist\tAlbum\n"+
		"missing fields\n"+
		"0\tNever played\tArtist\tAlbum\n"+
		"\n"+
		"2\tAlso good\tArtist\tAlbum\n", nil)

	got, err := r.PlayCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("kept %d rows, want the 2 readable ones: %+v", len(got), got)
	}
	if got[0].Title != "Good" || got[1].Title != "Also good" {
		t.Errorf("wrong rows survived: %+v", got)
	}
}

func TestPlayCounts_PropagatesAFailureToRead(t *testing.T) {
	// A listener on another platform, or with Music not installed, must get an
	// error the caller can degrade on — not an empty list that reads as "you
	// have never played anything".
	want := errors.New("osascript: not found")
	if _, err := readerReturning("", want).PlayCounts(context.Background()); !errors.Is(err, want) {
		t.Errorf("error not propagated: %v", err)
	}
}

func TestPlayCounts_EmptyLibrary(t *testing.T) {
	got, err := readerReturning("", nil).PlayCounts(context.Background())
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want empty and no error", got, err)
	}
}
