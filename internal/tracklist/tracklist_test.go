package tracklist

import (
	"strings"
	"testing"
)

func TestParseJSON(t *testing.T) {
	in := `{"name":"This Is X","track_count":2,"tracks":[
		{"position":1,"title":"Exagerado","artist":"Cazuza"},
		{"position":2,"title":"Preciso Dizer","artist":"Cazuza, Bebel Gilberto"}]}`
	name, got, err := ParseJSON(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if name != "This Is X" {
		t.Errorf("name = %q", name)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tracks", len(got))
	}
	if got[1].Title != "Preciso Dizer" || len(got[1].Artists) != 2 || got[1].Artists[0] != "Cazuza" || got[1].Artists[1] != "Bebel Gilberto" {
		t.Errorf("track 1 = %+v", got[1])
	}
}

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
