package apple

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// writeJSON writes a stub response, failing the test rather than the linter if
// the write cannot complete.
func writeJSON(t *testing.T, w io.Writer, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write stub response: %v", err)
	}
}

// newTestClient points a client at a stub catalog so batching, chunking and
// error handling can be exercised without touching the network.
func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewClient(Credentials{DeveloperToken: "dev", UserToken: "user", Storefront: "de"})
	c.apiBase = srv.URL
	return c
}

func TestSongAlbums_ResolvesEachSongToItsAlbum(t *testing.T) {
	// The point of this path: the album comes from the recording itself, so
	// there is no name matching and no edition guesswork to get wrong.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/catalog/de/songs") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(t, w, `{"data":[
			{"id":"1","relationships":{"albums":{"data":[{"id":"alb-A"}]}}},
			{"id":"2","relationships":{"albums":{"data":[{"id":"alb-A"}]}}},
			{"id":"3","relationships":{"albums":{"data":[{"id":"alb-B"}]}}}]}`)
	})

	got, err := c.SongAlbums(context.Background(), []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"1": "alb-A", "2": "alb-A", "3": "alb-B"}
	for id, album := range want {
		if got[id] != album {
			t.Errorf("song %s -> %q, want %q", id, got[id], album)
		}
	}
}

func TestSongAlbums_ChunksLargeRequests(t *testing.T) {
	// Apple caps ids per request; exceeding it fails the whole call, so a large
	// list must be split rather than truncated — a truncated batch would look
	// like "these songs have no album" and silently lose them.
	var batches [][]string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		batches = append(batches, ids)
		var b strings.Builder
		b.WriteString(`{"data":[`)
		for i, id := range ids {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":%q,"relationships":{"albums":{"data":[{"id":"alb-%s"}]}}}`, id, id)
		}
		b.WriteString(`]}`)
		writeJSON(t, w, b.String())
	})

	ids := make([]string, 60)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	got, err := c.SongAlbums(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 60 {
		t.Errorf("resolved %d songs, want all 60", len(got))
	}
	if len(batches) < 3 {
		t.Errorf("expected the request to be chunked, got %d batch(es)", len(batches))
	}
	for _, b := range batches {
		if len(b) > songBatch {
			t.Errorf("batch of %d exceeds the %d cap", len(b), songBatch)
		}
	}
}

func TestSongAlbums_OmitsSongsWithNoAlbum(t *testing.T) {
	// A song with no album relationship is simply unknown. Inventing an album
	// for it would put a track on a record it is not on.
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, `{"data":[
			{"id":"1","relationships":{"albums":{"data":[]}}},
			{"id":"2","relationships":{"albums":{"data":[{"id":"alb-B"}]}}}]}`)
	})

	got, err := c.SongAlbums(context.Background(), []string{"1", "2"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["1"]; ok {
		t.Error("a song with no album must not appear in the result")
	}
	if got["2"] != "alb-B" {
		t.Errorf("song 2 -> %q, want alb-B", got["2"])
	}
}

func TestSongAlbums_EmptyInputMakesNoRequest(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("no request should be made for an empty id list")
	})
	got, err := c.SongAlbums(context.Background(), nil)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v; want empty, nil", got, err)
	}
}

func TestAlbumsByID_ReturnsMetadataForEach(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/catalog/de/albums") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		writeJSON(t, w, `{"data":[
			{"id":"alb-A","attributes":{"name":"Is This It","artistName":"The Strokes",
				"trackCount":11,"upc":"00060","isCompilation":false,"genreNames":["Rock"]}},
			{"id":"alb-B","attributes":{"name":"Pop Anthems","artistName":"Various Artists",
				"trackCount":40,"upc":"00061","isCompilation":true,"genreNames":["Pop"]}}]}`)
	})

	got, err := c.AlbumsByID(context.Background(), []string{"alb-A", "alb-B"})
	if err != nil {
		t.Fatal(err)
	}
	a := got["alb-A"]
	if a.Name != "Is This It" || a.TrackCount != 11 || a.UPC != "00060" {
		t.Errorf("alb-A mapped wrong: %+v", a)
	}
	if a.IsCompilation {
		t.Error("alb-A is not a compilation")
	}
	if b := got["alb-B"]; !b.IsCompilation || b.TrackCount != 40 {
		t.Errorf("alb-B mapped wrong: %+v", b)
	}
}

func TestAlbumsByID_ChunksLargeRequests(t *testing.T) {
	var batches int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		batches++
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		if len(ids) > albumBatch {
			t.Errorf("batch of %d exceeds the %d cap", len(ids), albumBatch)
		}
		var b strings.Builder
		b.WriteString(`{"data":[`)
		for i, id := range ids {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"id":%q,"attributes":{"name":"A","trackCount":10}}`, id)
		}
		b.WriteString(`]}`)
		writeJSON(t, w, b.String())
	})

	ids := make([]string, 45)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	got, err := c.AlbumsByID(context.Background(), ids)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 45 {
		t.Errorf("resolved %d albums, want 45", len(got))
	}
	if batches < 2 {
		t.Errorf("expected chunking, got %d batch(es)", batches)
	}
}

func TestAlbumEditions_ReturnsEveryEditionSoTheCallerCanChoose(t *testing.T) {
	// The adapter finds editions; it must not decide between them. Choosing the
	// smallest here — before anyone knows how many tracks the listener loves —
	// is how "Wasting Light" became a two-track single while nine of its tracks
	// were being played.
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, `{"results":{"albums":{"data":[
			{"id":"single","attributes":{"name":"Wasting Light (Bonus Tracks) - Single","trackCount":2}},
			{"id":"album","attributes":{"name":"Wasting Light","trackCount":11}},
			{"id":"other","attributes":{"name":"Concrete and Gold","trackCount":11}}]}}}`)
	})

	got, err := c.AlbumEditions(context.Background(), "Wasting Light", "Foo Fighters")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want both editions of the record, got %d: %+v", len(got), got)
	}
	seen := map[string]int{}
	for _, a := range got {
		seen[a.ID] = a.TrackCount
	}
	if seen["single"] != 2 || seen["album"] != 11 {
		t.Errorf("editions mapped wrong: %v", seen)
	}
	if _, ok := seen["other"]; ok {
		t.Error("a different record must not be returned as an edition")
	}
}

func TestAlbumEditions_SearchesTheRecordNotTheEdition(t *testing.T) {
	// Searching for "moisturizer (deluxe)" biases the catalog toward returning
	// the deluxe, so the standard pressing never appears to be chosen from. The
	// query must name the record.
	var term string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		term = r.URL.Query().Get("term")
		writeJSON(t, w, `{"results":{"albums":{"data":[
			{"id":"std","attributes":{"name":"moisturizer","trackCount":12}}]}}}`)
	})

	if _, err := c.AlbumEditions(context.Background(), "moisturizer (deluxe)", "Wet Leg"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(term), "deluxe") {
		t.Errorf("query carried the edition qualifier: %q", term)
	}
}

func TestAlbumEditions_ReportsNotFoundDistinctly(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, `{"results":{}}`)
	})
	if _, err := c.AlbumEditions(context.Background(), "Nonexistent", "Nobody"); !errors.Is(err, domain.ErrAlbumNotFound) {
		t.Errorf("want ErrAlbumNotFound, got %v", err)
	}
}
