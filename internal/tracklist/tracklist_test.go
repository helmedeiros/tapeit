package tracklist

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	in := `# my list
Take Five - Dave Brubeck
So What — Miles Davis
Blue Rondo à la Turk | The Dave Brubeck Quartet

Naima
`
	got, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d tracks, want 4: %+v", len(got), got)
	}
	cases := []struct {
		title, artist string
	}{
		{"Take Five", "Dave Brubeck"},
		{"So What", "Miles Davis"},
		{"Blue Rondo à la Turk", "The Dave Brubeck Quartet"},
		{"Naima", ""},
	}
	for i, c := range cases {
		if got[i].Title != c.title {
			t.Errorf("track %d title = %q, want %q", i, got[i].Title, c.title)
		}
		artist := ""
		if len(got[i].Artists) > 0 {
			artist = got[i].Artists[0]
		}
		if artist != c.artist {
			t.Errorf("track %d artist = %q, want %q", i, artist, c.artist)
		}
	}
}
