package apple

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// albumDTO is the wire shape of a catalog album.
type albumDTO struct {
	ID         string `json:"id"`
	Attributes struct {
		Name          string `json:"name"`
		ArtistName    string `json:"artistName"`
		TrackCount    int    `json:"trackCount"`
		IsCompilation bool   `json:"isCompilation"`
	} `json:"attributes"`
}

type albumSearchResponse struct {
	Results struct {
		Albums struct {
			Data []albumDTO `json:"data"`
		} `json:"albums"`
	} `json:"results"`
}

// Album implements domain.AlbumPort.
//
// Apple lists a record many times over — standard, deluxe, anniversary, plus
// territory variants. Coverage (how much of a record a listener loves) is
// measured against the track count, so picking a 19-track deluxe when the album
// is really 12 tracks silently understates devotion by a third. This deliberately
// keeps the *smallest* edition whose name matches, which is also the one you
// would actually buy on vinyl.
func (c *Client) Album(ctx context.Context, name, artist string) (domain.Album, error) {
	if c.creds.Storefront == "" {
		return domain.Album{}, fmt.Errorf("storefront not set")
	}
	q := url.Values{
		"types": {"albums"},
		"term":  {name + " " + artist},
		"limit": {"10"},
	}
	u := fmt.Sprintf("%s/catalog/%s/search?%s", apiBase, c.creds.Storefront, q.Encode())

	var resp albumSearchResponse
	if err := c.do(ctx, http.MethodGet, u, nil, true, &resp); err != nil {
		return domain.Album{}, err
	}

	want := domain.BaseAlbumName(name)
	var best *albumDTO
	for i := range resp.Results.Albums.Data {
		d := &resp.Results.Albums.Data[i]
		if domain.BaseAlbumName(d.Attributes.Name) != want {
			continue
		}
		if best == nil || d.Attributes.TrackCount < best.Attributes.TrackCount {
			best = d
		}
	}
	if best == nil {
		return domain.Album{}, fmt.Errorf("album %q by %q: %w", name, artist, domain.ErrAlbumNotFound)
	}

	return domain.Album{
		ID:            best.ID,
		Name:          best.Attributes.Name,
		Artist:        best.Attributes.ArtistName,
		TrackCount:    best.Attributes.TrackCount,
		IsCompilation: best.Attributes.IsCompilation,
	}, nil
}

// AlbumRuntime implements domain.AlbumPort. The catalog exposes duration only
// per track, so an album's runtime costs a request of its own.
func (c *Client) AlbumRuntime(ctx context.Context, albumID string) (int, error) {
	if c.creds.Storefront == "" {
		return 0, fmt.Errorf("storefront not set")
	}
	var tracks songsResponse
	u := fmt.Sprintf("%s/catalog/%s/albums/%s/tracks?limit=100", apiBase, c.creds.Storefront, albumID)
	if err := c.do(ctx, http.MethodGet, u, nil, true, &tracks); err != nil {
		return 0, err
	}
	total := 0
	for _, t := range tracks.Data {
		total += t.Attributes.DurationMillis
	}
	return total, nil
}
