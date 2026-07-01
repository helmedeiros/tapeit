// Package deezer reads track metadata from the public Deezer API
// (api.deezer.com), which needs no authentication and exposes fields Apple and
// the current Spotify API do not — notably tempo (bpm) and an ISRC. It is used
// to enrich the canonical playlist lists.
package deezer

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
)

// apiBase is a var so tests can point it at a stub server.
var apiBase = "https://api.deezer.com"

// minInterval paces requests; Deezer allows ~50 requests per 5s.
const minInterval = 250 * time.Millisecond

// Track is the subset of Deezer's track fields we use.
type Track struct {
	ID          int64
	Title       string
	Artist      string
	Album       string
	DurationSec int
	ISRC        string
	BPM         float64
	Gain        float64
}

// Client queries the Deezer API, self-throttling to respect its rate limit.
type Client struct {
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Deezer API client with sane timeouts and self-throttling.
func NewClient() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("deezer: %s (code %d)", e.Message, e.Code)
}

type trackDTO struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Duration int     `json:"duration"`
	ISRC     string  `json:"isrc"`
	BPM      float64 `json:"bpm"`
	Gain     float64 `json:"gain"`
	Artist   struct {
		Name string `json:"name"`
	} `json:"artist"`
	Album struct {
		Title string `json:"title"`
	} `json:"album"`
	Error *apiError `json:"error"`
}

func (d trackDTO) toTrack() Track {
	return Track{
		ID:          d.ID,
		Title:       d.Title,
		Artist:      d.Artist.Name,
		Album:       d.Album.Title,
		DurationSec: d.Duration,
		ISRC:        d.ISRC,
		BPM:         d.BPM,
		Gain:        d.Gain,
	}
}

// Search returns candidate tracks matching title + artist. The search results
// carry ISRC and duration but not bpm; fetch bpm with Track.
func (c *Client) Search(ctx context.Context, title, artist string) ([]Track, error) {
	q := fmt.Sprintf(`artist:"%s" track:"%s"`, artist, title)
	u := apiBase + "/search/track?limit=10&q=" + url.QueryEscape(q)

	var body struct {
		Data  []trackDTO `json:"data"`
		Error *apiError  `json:"error"`
	}
	if err := c.get(ctx, u, &body); err != nil {
		return nil, err
	}
	if body.Error != nil {
		return nil, body.Error
	}
	out := make([]Track, 0, len(body.Data))
	for _, d := range body.Data {
		out = append(out, d.toTrack())
	}
	return out, nil
}

// Track fetches a full track by id, which includes bpm and gain.
func (c *Client) Track(ctx context.Context, id int64) (Track, error) {
	var d trackDTO
	if err := c.get(ctx, apiBase+"/track/"+strconv.FormatInt(id, 10), &d); err != nil {
		return Track{}, err
	}
	if d.Error != nil {
		return Track{}, d.Error
	}
	return d.toTrack(), nil
}

// get paces, fetches, and decodes. Deezer signals quota errors in a 200 body
// (code 4); retry those a few times with backoff.
func (c *Client) get(ctx context.Context, u string, out any) error {
	for attempt := 0; ; attempt++ {
		if err := c.pace(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("deezer request: %w", err)
		}
		buf, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("deezer read: %w", err)
		}

		var probe struct {
			Error *apiError `json:"error"`
		}
		_ = json.Unmarshal(buf, &probe)
		if probe.Error != nil && probe.Error.Code == 4 && attempt < 4 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
			continue
		}
		if err := json.Unmarshal(buf, out); err != nil {
			return fmt.Errorf("deezer decode: %w", err)
		}
		return nil
	}
}

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
