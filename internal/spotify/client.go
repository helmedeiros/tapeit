package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const apiBase = "https://api.spotify.com/v1"

// TokenStore persists a refreshed token so the latest is reused next run.
type TokenStore func(Token) error

// Client talks to the Spotify Web API, transparently refreshing the token and
// honoring rate limits.
type Client struct {
	http *http.Client
	mu   sync.Mutex
	tok  Token
	save TokenStore
}

// NewClient builds a client from an existing token. save is called whenever the
// token is refreshed (may be nil to skip persistence).
func NewClient(tok Token, save TokenStore) *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, tok: tok, save: save}
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tok.expired() {
		nt, err := refreshToken(ctx, c.tok)
		if err != nil {
			return "", fmt.Errorf("refresh token: %w", err)
		}
		c.tok = nt
		if c.save != nil {
			if err := c.save(nt); err != nil {
				return "", fmt.Errorf("persist refreshed token: %w", err)
			}
		}
	}
	return c.tok.AccessToken, nil
}

// getJSON issues a GET against an absolute URL (or apiBase-relative path),
// decoding the JSON body into out. It retries on 429 and refreshes once on 401.
func (c *Client) getJSON(ctx context.Context, url string, out any) error {
	if url != "" && url[0] == '/' {
		url = apiBase + url
	}
	for attempt := 0; ; attempt++ {
		tok, err := c.accessToken(ctx)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("GET %s: %w", url, err)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			err := json.NewDecoder(resp.Body).Decode(out)
			_ = resp.Body.Close()
			if err != nil {
				return fmt.Errorf("decode %s: %w", url, err)
			}
			return nil
		case http.StatusTooManyRequests:
			wait := retryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		case http.StatusUnauthorized:
			_ = resp.Body.Close()
			if attempt == 0 {
				c.forceExpire()
				continue
			}
			return fmt.Errorf("GET %s: unauthorized after refresh", url)
		default:
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
			_ = resp.Body.Close()
			return fmt.Errorf("GET %s: status %d: %s", url, resp.StatusCode, string(b))
		}
	}
}

func (c *Client) forceExpire() {
	c.mu.Lock()
	c.tok.Expiry = time.Time{}
	c.mu.Unlock()
}

func retryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(h); err == nil && n >= 0 {
		return time.Duration(n)*time.Second + 500*time.Millisecond
	}
	return 2 * time.Second
}

// --- API DTOs (wire shapes, never leak past the spotify package) ---

type meDTO struct {
	ID string `json:"id"`
}

type playlistsPage struct {
	Items []playlistDTO `json:"items"`
	Next  string        `json:"next"`
}

type playlistDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Owner       struct {
		ID string `json:"id"`
	} `json:"owner"`
}

type tracksPage struct {
	Items []struct {
		Track *trackDTO `json:"track"`
	} `json:"items"`
	Next string `json:"next"`
}

type trackDTO struct {
	ID         string `json:"id"`
	URI        string `json:"uri"`
	Name       string `json:"name"`
	DurationMS int    `json:"duration_ms"`
	Type       string `json:"type"`
	Album      struct {
		Name string `json:"name"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	ExternalIDs struct {
		ISRC string `json:"isrc"`
	} `json:"external_ids"`
}

// currentUserID returns the authenticated user's Spotify ID.
func (c *Client) currentUserID(ctx context.Context) (string, error) {
	var me meDTO
	if err := c.getJSON(ctx, "/me", &me); err != nil {
		return "", err
	}
	return me.ID, nil
}

// userPlaylists returns all playlists owned or followed by the user.
func (c *Client) userPlaylists(ctx context.Context) ([]playlistDTO, error) {
	var all []playlistDTO
	next := "/me/playlists?limit=50"
	for next != "" {
		var page playlistsPage
		if err := c.getJSON(ctx, next, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		next = page.Next
	}
	return all, nil
}

// playlistTracks returns the tracks of a playlist, following pagination. The
// Feb 2026 API renamed /tracks to /items; we try /tracks then fall back.
func (c *Client) playlistTracks(ctx context.Context, playlistID string) ([]trackDTO, error) {
	tracks, err := c.pagedTracks(ctx, "/playlists/"+playlistID+"/tracks?limit=100")
	if err != nil {
		if alt, altErr := c.pagedTracks(ctx, "/playlists/"+playlistID+"/items?limit=100"); altErr == nil {
			return alt, nil
		}
		return nil, err
	}
	return tracks, nil
}

// savedTracks returns the user's Liked Songs.
func (c *Client) savedTracks(ctx context.Context) ([]trackDTO, error) {
	return c.pagedTracks(ctx, "/me/tracks?limit=50")
}

func (c *Client) pagedTracks(ctx context.Context, first string) ([]trackDTO, error) {
	var all []trackDTO
	next := first
	for next != "" {
		var page tracksPage
		if err := c.getJSON(ctx, next, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			if it.Track == nil || (it.Track.Type != "" && it.Track.Type != "track") {
				continue // removed item or a podcast episode
			}
			all = append(all, *it.Track)
		}
		next = page.Next
	}
	return all, nil
}
