package deezer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchAndTrack(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/track"):
			_, _ = w.Write([]byte(`{"data":[{"id":42,"title":"Lonely Boy","duration":193,"isrc":"USNO11100273","artist":{"name":"The Black Keys"},"album":{"title":"El Camino"}}]}`))
		case r.URL.Path == "/track/42":
			_, _ = w.Write([]byte(`{"id":42,"title":"Lonely Boy","duration":193,"isrc":"USNO11100273","bpm":168.1,"gain":-10.8,"artist":{"name":"The Black Keys"},"album":{"title":"El Camino"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	orig := apiBase
	apiBase = srv.URL
	defer func() { apiBase = orig }()

	cands, err := c.Search(context.Background(), "Lonely Boy", "The Black Keys")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(cands) != 1 || cands[0].ISRC != "USNO11100273" || cands[0].DurationSec != 193 {
		t.Fatalf("bad search result: %+v", cands)
	}

	full, err := c.Track(context.Background(), cands[0].ID)
	if err != nil {
		t.Fatalf("track: %v", err)
	}
	if full.BPM != 168.1 || full.Gain != -10.8 {
		t.Errorf("want bpm 168.1 gain -10.8, got %+v", full)
	}
}

func TestQuotaErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"error":{"code":800,"message":"no data","type":"DataException"}}`))
	}))
	defer srv.Close()

	c := &Client{http: srv.Client()}
	orig := apiBase
	apiBase = srv.URL
	defer func() { apiBase = orig }()

	if _, err := c.Search(context.Background(), "x", "y"); err == nil {
		t.Fatal("expected an error from a Deezer error body")
	}
}
