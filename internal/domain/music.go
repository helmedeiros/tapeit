package domain

// PlaylistKind distinguishes how a playlist relates to the user.
type PlaylistKind string

const (
	// Owned is a playlist the user created.
	Owned PlaylistKind = "owned"
	// Followed is a playlist the user follows but does not own.
	Followed PlaylistKind = "followed"
	// LikedSongs is the user's saved/liked tracks, modeled as a playlist.
	LikedSongs PlaylistKind = "liked_songs"
)

// Track is a single recording as seen on the source side. It carries enough to
// match against a target catalog: ISRC is the primary key, the rest is fallback.
type Track struct {
	Title      string   `json:"title"`
	Artists    []string `json:"artists"`
	Album      string   `json:"album"`
	DurationMS int      `json:"duration_ms"`
	ISRC       string   `json:"isrc,omitempty"`
	SpotifyID  string   `json:"spotify_id,omitempty"`
	SpotifyURI string   `json:"spotify_uri,omitempty"`
}

// Playlist is a named, ordered collection of tracks.
type Playlist struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Kind        PlaylistKind `json:"kind"`
	OwnerID     string       `json:"owner_id,omitempty"`
	SpotifyID   string       `json:"spotify_id,omitempty"`
	Tracks      []Track      `json:"tracks"`
}
