// Package itunes matches tracks via the public iTunes Search API
// (itunes.apple.com/search). It implements domain.CatalogPort and exists as an
// alternative to the amp-api catalog search, which shares a tight rate limit
// with the rest of the Apple Music API; the iTunes Search API has its own,
// separate quota and returns a trackId that is a valid Apple Music catalog id.
package itunes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/helmedeiros/tapeit/internal/domain"
)

const searchURL = "https://itunes.apple.com/search"

// minInterval paces requests; the iTunes Search API tolerates roughly 20/min.
const minInterval = 3 * time.Second

// Client queries the iTunes Search API, self-throttling to respect its limit.
type Client struct {
	http       *http.Client
	storefront string
	mu         sync.Mutex
	last       time.Time
}

// NewClient builds a client for the given storefront (ISO country, e.g. "de").
func NewClient(storefront string) *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, storefront: storefront}
}

// SongsByISRC implements domain.CatalogPort. The iTunes Search API has no
// reliable ISRC lookup, so this returns nothing and the matcher falls back to
// SearchSongs.
func (c *Client) SongsByISRC(context.Context, []string) (map[string][]domain.CatalogSong, error) {
	return map[string][]domain.CatalogSong{}, nil
}

type result struct {
	TrackID        int64  `json:"trackId"`
	TrackName      string `json:"trackName"`
	ArtistName     string `json:"artistName"`
	CollectionName string `json:"collectionName"`
	TrackTimeMS    int    `json:"trackTimeMillis"`
}

// SearchSongs implements domain.CatalogPort via the iTunes Search API.
func (c *Client) SearchSongs(ctx context.Context, term string, limit int) ([]domain.CatalogSong, error) {
	q := url.Values{
		"term":    {term},
		"entity":  {"song"},
		"limit":   {strconv.Itoa(limit)},
		"country": {c.storefront},
	}
	u := searchURL + "?" + q.Encode()

	for attempt := 0; ; attempt++ {
		if err := c.pace(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("itunes search: %w", err)
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			_ = resp.Body.Close()
			if attempt >= 5 {
				return nil, fmt.Errorf("itunes search rate-limited (status %d)", resp.StatusCode)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 5 * time.Second):
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("itunes search: status %d: %s", resp.StatusCode, string(b))
		}
		var body struct {
			Results []result `json:"results"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("itunes decode: %w", err)
		}
		songs := make([]domain.CatalogSong, 0, len(body.Results))
		for _, r := range body.Results {
			songs = append(songs, domain.CatalogSong{
				ID:         strconv.FormatInt(r.TrackID, 10),
				Title:      r.TrackName,
				Artist:     r.ArtistName,
				Album:      r.CollectionName,
				DurationMS: r.TrackTimeMS,
			})
		}
		return songs, nil
	}
}

// pace blocks until at least minInterval has elapsed since the previous request.
func (c *Client) pace(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() {
		if d := minInterval - time.Since(c.last); d > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d):
			}
		}
	}
	c.last = time.Now()
	return nil
}

var _ domain.CatalogPort = (*Client)(nil)
