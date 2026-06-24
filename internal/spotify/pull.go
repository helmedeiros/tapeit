package spotify

import (
	"context"
	"fmt"

	"github.com/helmedeiros/tapeit/internal/domain"
	"github.com/helmedeiros/tapeit/internal/snapshot"
)

// PullOptions controls what the pull captures.
type PullOptions struct {
	// OwnedOnly skips playlists the user follows but does not own.
	OwnedOnly bool
	// Progress, if set, is called with human-readable progress lines.
	Progress func(string)
}

func (o PullOptions) report(format string, args ...any) {
	if o.Progress != nil {
		o.Progress(fmt.Sprintf(format, args...))
	}
}

// Pull downloads the user's playlists (owned + followed) and Liked Songs into a
// snapshot. It is the only stage that requires a live Spotify connection.
func (c *Client) Pull(ctx context.Context, opts PullOptions) (snapshot.Library, error) {
	userID, err := c.currentUserID(ctx)
	if err != nil {
		return snapshot.Library{}, fmt.Errorf("identify user: %w", err)
	}
	opts.report("authenticated as %s", userID)

	metas, err := c.userPlaylists(ctx)
	if err != nil {
		return snapshot.Library{}, fmt.Errorf("list playlists: %w", err)
	}
	opts.report("found %d playlists", len(metas))

	var playlists []domain.Playlist
	for _, m := range metas {
		kind := domain.Owned
		if m.Owner.ID != userID {
			if opts.OwnedOnly {
				continue
			}
			kind = domain.Followed
		}
		tracks, err := c.playlistTracks(ctx, m.ID)
		if err != nil {
			return snapshot.Library{}, fmt.Errorf("tracks for %q: %w", m.Name, err)
		}
		playlists = append(playlists, domain.Playlist{
			Name:        m.Name,
			Description: m.Description,
			Kind:        kind,
			OwnerID:     m.Owner.ID,
			SpotifyID:   m.ID,
			Tracks:      mapTracks(tracks),
		})
		opts.report("  %-40s %s (%d tracks)", truncate(m.Name, 40), kind, len(tracks))
	}

	liked, err := c.savedTracks(ctx)
	if err != nil {
		return snapshot.Library{}, fmt.Errorf("liked songs: %w", err)
	}
	playlists = append(playlists, domain.Playlist{
		Name:   "Liked Songs (from Spotify)",
		Kind:   domain.LikedSongs,
		Tracks: mapTracks(liked),
	})
	opts.report("  Liked Songs: %d tracks", len(liked))

	return snapshot.Library{SpotifyUserID: userID, Playlists: playlists}, nil
}

func mapTracks(dtos []trackDTO) []domain.Track {
	out := make([]domain.Track, 0, len(dtos))
	for _, d := range dtos {
		out = append(out, mapTrack(d))
	}
	return out
}

func mapTrack(d trackDTO) domain.Track {
	artists := make([]string, 0, len(d.Artists))
	for _, a := range d.Artists {
		artists = append(artists, a.Name)
	}
	return domain.Track{
		Title:      d.Name,
		Artists:    artists,
		Album:      d.Album.Name,
		DurationMS: d.DurationMS,
		ISRC:       d.ExternalIDs.ISRC,
		SpotifyID:  d.ID,
		SpotifyURI: d.URI,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
