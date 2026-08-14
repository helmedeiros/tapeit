package apple

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
)

const (
	// songBatch and albumBatch are how many ids fit in one request. Apple caps
	// this, and exceeding the cap fails the whole call — which would look like
	// "these songs have no album" and silently lose them — so requests are
	// chunked rather than truncated.
	songBatch  = 25
	albumBatch = 25
)

// SongAlbums implements domain.AlbumPort.
//
// This is the exact path, and the reason it exists: the album comes from the
// recording itself, so there is no name matching, no edition guesswork and no
// storefront language to trip over. A song the catalog reports no album for is
// simply absent from the result — inventing one would place a track on a record
// it is not on.
func (c *Client) SongAlbums(ctx context.Context, songIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(songIDs))
	if err := c.eachBatch(ctx, songIDs, songBatch, "songs?include=albums", func(raw json.RawMessage) error {
		var resp struct {
			Data []struct {
				ID            string `json:"id"`
				Relationships struct {
					Albums struct {
						Data []struct {
							ID string `json:"id"`
						} `json:"data"`
					} `json:"albums"`
				} `json:"relationships"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		for _, s := range resp.Data {
			if len(s.Relationships.Albums.Data) > 0 {
				out[s.ID] = s.Relationships.Albums.Data[0].ID
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// AlbumsByID implements domain.AlbumPort, resolving albums by their catalog id.
func (c *Client) AlbumsByID(ctx context.Context, albumIDs []string) (map[string]domain.Album, error) {
	out := make(map[string]domain.Album, len(albumIDs))
	if err := c.eachBatch(ctx, albumIDs, albumBatch, "albums", func(raw json.RawMessage) error {
		var resp struct {
			Data []albumDTO `json:"data"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			return err
		}
		for _, a := range resp.Data {
			out[a.ID] = a.toDomain()
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// eachBatch splits ids into requests of at most size and hands each response
// body to decode. path may carry query parameters; "ids" is appended.
func (c *Client) eachBatch(ctx context.Context, ids []string, size int, path string,
	decode func(json.RawMessage) error) error {
	if c.creds.Storefront == "" {
		return fmt.Errorf("storefront not set")
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for start := 0; start < len(ids); start += size {
		end := min(start+size, len(ids))
		u := fmt.Sprintf("%s/catalog/%s/%s%sids=%s",
			c.apiBase, c.creds.Storefront, path, sep, url.QueryEscape(strings.Join(ids[start:end], ",")))
		var raw json.RawMessage
		if err := c.do(ctx, http.MethodGet, u, nil, true, &raw); err != nil {
			return err
		}
		if err := decode(raw); err != nil {
			return err
		}
	}
	return nil
}

// albumDTO is the wire shape of a catalog album.
type albumDTO struct {
	ID         string `json:"id"`
	Attributes struct {
		Name          string   `json:"name"`
		ArtistName    string   `json:"artistName"`
		TrackCount    int      `json:"trackCount"`
		IsCompilation bool     `json:"isCompilation"`
		UPC           string   `json:"upc"`
		GenreNames    []string `json:"genreNames"`
	} `json:"attributes"`
}

// toDomain maps the wire shape to the domain's album.
func (d albumDTO) toDomain() domain.Album {
	return domain.Album{
		ID:            d.ID,
		Name:          d.Attributes.Name,
		Artist:        d.Attributes.ArtistName,
		TrackCount:    d.Attributes.TrackCount,
		UPC:           d.Attributes.UPC,
		Genres:        d.Attributes.GenreNames,
		IsCompilation: d.Attributes.IsCompilation,
	}
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
	u := fmt.Sprintf("%s/catalog/%s/search?%s", c.apiBase, c.creds.Storefront, q.Encode())

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

	return best.toDomain(), nil
}

// AlbumRuntime implements domain.AlbumPort. The catalog exposes duration only
// per track, so an album's runtime costs a request of its own.
func (c *Client) AlbumRuntime(ctx context.Context, albumID string) (int, error) {
	if c.creds.Storefront == "" {
		return 0, fmt.Errorf("storefront not set")
	}
	var tracks songsResponse
	u := fmt.Sprintf("%s/catalog/%s/albums/%s/tracks?limit=100", c.apiBase, c.creds.Storefront, albumID)
	if err := c.do(ctx, http.MethodGet, u, nil, true, &tracks); err != nil {
		return 0, err
	}
	total := 0
	for _, t := range tracks.Data {
		total += t.Attributes.DurationMillis
	}
	return total, nil
}
