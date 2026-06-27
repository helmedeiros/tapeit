package apple

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// errNotFound marks a 404 so callers can treat it as "empty" where that is the
// expected meaning (Apple returns 404 for an empty playlist's tracks).
var errNotFound = errors.New("not found")

const (
	apiBase = "https://amp-api.music.apple.com/v1"
	origin  = "https://music.apple.com"

	// addBatch is the defensive chunk size for adding tracks to a playlist.
	addBatch = 100

	// maxRetries is how many times to retry a 429 before giving up.
	maxRetries = 8
)

// Client calls the Apple Music API. It implements domain.CatalogPort and
// domain.LibraryPort.
type Client struct {
	http  *http.Client
	creds Credentials
}

// NewClient builds an Apple Music client from extracted credentials.
func NewClient(creds Credentials) *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, creds: creds}
}

func (c *Client) setHeaders(req *http.Request, withUser bool) {
	req.Header.Set("Authorization", "Bearer "+c.creds.DeveloperToken)
	req.Header.Set("Origin", origin)
	if withUser && c.creds.UserToken != "" {
		req.Header.Set("Music-User-Token", c.creds.UserToken)
	}
}

// do issues a request with retry on 429 and decodes a JSON body into out.
func (c *Client) do(ctx context.Context, method, url string, body []byte, withUser bool, out any) error {
	for attempt := 0; ; attempt++ {
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rdr)
		if err != nil {
			return err
		}
		c.setHeaders(req, withUser)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, url, err)
		}

		// Retry on rate limiting and transient upstream errors. Apple's Cloud
		// Library intermittently returns 500/503; these generally do not commit
		// a write, so retrying the same add is safe in practice.
		if (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500) && attempt < maxRetries {
			wait := backoff(resp.Header.Get("Retry-After"), attempt)
			_ = resp.Body.Close()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
			_ = resp.Body.Close()
			return apiError(method, url, resp.StatusCode, string(b))
		}
		if out == nil {
			_ = resp.Body.Close()
			return nil
		}
		err = json.NewDecoder(resp.Body).Decode(out)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("decode %s: %w", url, err)
		}
		return nil
	}
}

func apiError(method, url string, status int, body string) error {
	switch status {
	case http.StatusNotFound:
		return fmt.Errorf("%s %s: %w", method, url, errNotFound)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%s %s: %d unauthorized — your Apple tokens may have expired; re-run `tapeit auth apple` (%s)", method, url, status, body)
	default:
		return fmt.Errorf("%s %s: status %d: %s", method, url, status, body)
	}
}

// backoff honors a Retry-After header when present, else uses exponential
// backoff (1s, 2s, 4s, … capped at 30s).
func backoff(retryAfter string, attempt int) time.Duration {
	if n, err := strconv.Atoi(retryAfter); err == nil && n >= 0 {
		return time.Duration(n)*time.Second + 500*time.Millisecond
	}
	d := time.Duration(1<<attempt) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// --- wire DTOs ---

type songsResponse struct {
	Data []songDTO `json:"data"`
	Next string    `json:"next"`
}

type searchResponse struct {
	Results struct {
		Songs struct {
			Data []songDTO `json:"data"`
		} `json:"songs"`
	} `json:"results"`
}

type songDTO struct {
	ID         string `json:"id"`
	Attributes struct {
		Name           string `json:"name"`
		ArtistName     string `json:"artistName"`
		AlbumName      string `json:"albumName"`
		DurationMillis int    `json:"durationInMillis"`
		ISRC           string `json:"isrc"`
	} `json:"attributes"`
}

func (d songDTO) toDomain() domain.CatalogSong {
	return domain.CatalogSong{
		ID:         d.ID,
		Title:      d.Attributes.Name,
		Artist:     d.Attributes.ArtistName,
		Album:      d.Attributes.AlbumName,
		DurationMS: d.Attributes.DurationMillis,
		ISRC:       d.Attributes.ISRC,
	}
}

// Storefront resolves the user's storefront id (e.g. "de"). Requires the user
// token.
func (c *Client) Storefront(ctx context.Context) (string, error) {
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, apiBase+"/me/storefront", nil, true, &out); err != nil {
		return "", err
	}
	if len(out.Data) == 0 {
		return "", fmt.Errorf("storefront: empty response")
	}
	return out.Data[0].ID, nil
}

// SongsByISRC implements domain.CatalogPort.
func (c *Client) SongsByISRC(ctx context.Context, isrcs []string) (map[string][]domain.CatalogSong, error) {
	if c.creds.Storefront == "" {
		return nil, fmt.Errorf("storefront not set")
	}
	result := make(map[string][]domain.CatalogSong)
	next := fmt.Sprintf("%s/catalog/%s/songs?filter[isrc]=%s", apiBase, c.creds.Storefront, strings.Join(isrcs, ","))
	for next != "" {
		var resp songsResponse
		// Send the user token: authenticated catalog reads get a much higher
		// rate limit than the shared anonymous developer-token quota.
		if err := c.do(ctx, http.MethodGet, next, nil, true, &resp); err != nil {
			return nil, err
		}
		for _, s := range resp.Data {
			key := strings.ToUpper(s.Attributes.ISRC)
			result[key] = append(result[key], s.toDomain())
		}
		next = absolute(resp.Next)
	}
	return result, nil
}

// SearchSongs implements domain.CatalogPort.
func (c *Client) SearchSongs(ctx context.Context, term string, limit int) ([]domain.CatalogSong, error) {
	if c.creds.Storefront == "" {
		return nil, fmt.Errorf("storefront not set")
	}
	q := url.Values{
		"types": {"songs"},
		"term":  {term},
		"limit": {strconv.Itoa(limit)},
	}
	u := fmt.Sprintf("%s/catalog/%s/search?%s", apiBase, c.creds.Storefront, q.Encode())
	var resp searchResponse
	// Authenticated search avoids the tight anonymous developer-token rate limit.
	if err := c.do(ctx, http.MethodGet, u, nil, true, &resp); err != nil {
		return nil, err
	}
	songs := make([]domain.CatalogSong, 0, len(resp.Results.Songs.Data))
	for _, s := range resp.Results.Songs.Data {
		songs = append(songs, s.toDomain())
	}
	return songs, nil
}

// ExistingPlaylists implements domain.LibraryPort.
func (c *Client) ExistingPlaylists(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string)
	next := apiBase + "/me/library/playlists?limit=100"
	for next != "" {
		var resp struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
			Next string `json:"next"`
		}
		if err := c.do(ctx, http.MethodGet, next, nil, true, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp.Data {
			out[p.Attributes.Name] = p.ID
		}
		next = absolute(resp.Next)
	}
	return out, nil
}

// CreatePlaylist implements domain.LibraryPort.
func (c *Client) CreatePlaylist(ctx context.Context, name, description string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"attributes": map[string]string{"name": name, "description": description},
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodPost, apiBase+"/me/library/playlists", body, true, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("create playlist %q: empty response", name)
	}
	return resp.Data[0].ID, nil
}

// PlaylistTrackRefs implements domain.LibraryPort, reading each track's
// title+artist (reliable, unlike catalog ids). Apple returns 404 for an empty
// playlist; treat that as no tracks.
func (c *Client) PlaylistTrackRefs(ctx context.Context, playlistID string) ([]domain.TrackRef, error) {
	var refs []domain.TrackRef
	next := fmt.Sprintf("%s/me/library/playlists/%s/tracks?limit=100", apiBase, playlistID)
	for next != "" {
		var resp struct {
			Data []struct {
				Attributes struct {
					Name       string `json:"name"`
					ArtistName string `json:"artistName"`
				} `json:"attributes"`
			} `json:"data"`
			Next string `json:"next"`
		}
		if err := c.do(ctx, http.MethodGet, next, nil, true, &resp); err != nil {
			if errors.Is(err, errNotFound) {
				return refs, nil
			}
			return nil, err
		}
		for _, t := range resp.Data {
			refs = append(refs, domain.TrackRef{Title: t.Attributes.Name, Artist: t.Attributes.ArtistName})
		}
		next = absolute(resp.Next)
	}
	return refs, nil
}

// LibraryTrack is a track in a library playlist with its catalog metadata.
type LibraryTrack struct {
	Title      string
	Artist     string
	Album      string
	DurationMS int
	CatalogID  string
}

// PlaylistTracks returns the tracks of a library playlist.
func (c *Client) PlaylistTracks(ctx context.Context, playlistID string) ([]LibraryTrack, error) {
	var tracks []LibraryTrack
	next := fmt.Sprintf("%s/me/library/playlists/%s/tracks?limit=100", apiBase, playlistID)
	for next != "" {
		var resp struct {
			Data []struct {
				Attributes struct {
					Name           string `json:"name"`
					ArtistName     string `json:"artistName"`
					AlbumName      string `json:"albumName"`
					DurationMillis int    `json:"durationInMillis"`
					PlayParams     struct {
						CatalogID string `json:"catalogId"`
					} `json:"playParams"`
				} `json:"attributes"`
			} `json:"data"`
			Next string `json:"next"`
		}
		if err := c.do(ctx, http.MethodGet, next, nil, true, &resp); err != nil {
			if errors.Is(err, errNotFound) {
				return tracks, nil
			}
			return tracks, err // partial pages survive a mid-pagination failure
		}
		for _, t := range resp.Data {
			tracks = append(tracks, LibraryTrack{
				Title:      t.Attributes.Name,
				Artist:     t.Attributes.ArtistName,
				Album:      t.Attributes.AlbumName,
				DurationMS: t.Attributes.DurationMillis,
				CatalogID:  t.Attributes.PlayParams.CatalogID,
			})
		}
		next = absolute(resp.Next)
	}
	return tracks, nil
}

// AddTracks implements domain.LibraryPort, chunking to stay within limits.
func (c *Client) AddTracks(ctx context.Context, playlistID string, songIDs []string) error {
	u := fmt.Sprintf("%s/me/library/playlists/%s/tracks", apiBase, playlistID)
	for start := 0; start < len(songIDs); start += addBatch {
		end := min(start+addBatch, len(songIDs))
		data := make([]map[string]string, 0, end-start)
		for _, id := range songIDs[start:end] {
			data = append(data, map[string]string{"id": id, "type": "songs"})
		}
		body, err := json.Marshal(map[string]any{"data": data})
		if err != nil {
			return err
		}
		if err := c.do(ctx, http.MethodPost, u, body, true, nil); err != nil {
			return err
		}
	}
	return nil
}

// absolute turns a relative API "next" path into a full URL.
func absolute(next string) string {
	if next == "" {
		return ""
	}
	if strings.HasPrefix(next, "http") {
		return next
	}
	if strings.HasPrefix(next, "/v1/") {
		return "https://amp-api.music.apple.com" + next
	}
	return apiBase + "/" + strings.TrimPrefix(next, "/")
}

var (
	_ domain.CatalogPort = (*Client)(nil)
	_ domain.LibraryPort = (*Client)(nil)
)
