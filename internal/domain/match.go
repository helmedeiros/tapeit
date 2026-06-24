package domain

import "context"

// Confidence describes how sure we are that a Match is correct.
type Confidence string

const (
	// ConfExact is an ISRC-based match (highest trust).
	ConfExact Confidence = "exact"
	// ConfHigh is a strong text match (title + artist + duration).
	ConfHigh Confidence = "high"
	// ConfLow is a weak text match that may warrant manual review.
	ConfLow Confidence = "low"
	// ConfNone means no acceptable target was found.
	ConfNone Confidence = "none"
)

// MatchMethod records how a Match was produced.
type MatchMethod string

const (
	// MethodISRC matched via ISRC catalog lookup.
	MethodISRC MatchMethod = "isrc"
	// MethodSearch matched via catalog text search.
	MethodSearch MatchMethod = "search"
	// MethodNone means no match.
	MethodNone MatchMethod = "none"
)

// Match links a source Track to a target-catalog song.
type Match struct {
	Track      Track       `json:"track"`
	AppleID    string      `json:"apple_id,omitempty"`
	Confidence Confidence  `json:"confidence"`
	Method     MatchMethod `json:"method"`
	Note       string      `json:"note,omitempty"`
}

// Matched reports whether the track resolved to a target song.
func (m Match) Matched() bool { return m.AppleID != "" }

// CatalogSong is a target-catalog (Apple Music) song, as the domain sees it.
type CatalogSong struct {
	ID         string
	Title      string
	Artist     string
	Album      string
	DurationMS int
	ISRC       string
}

// CatalogPort reads the target music catalog. The storefront is a property of
// the adapter, not the domain.
type CatalogPort interface {
	// SongsByISRC returns candidate songs keyed by the (upper-cased) ISRC.
	SongsByISRC(ctx context.Context, isrcs []string) (map[string][]CatalogSong, error)
	// SearchSongs returns catalog songs matching a free-text term.
	SearchSongs(ctx context.Context, term string, limit int) ([]CatalogSong, error)
}

// TrackRef is a lightweight identity for a track already in the library, used
// to diff a playlist by title+artist. (Catalog ids are NOT used for this:
// Apple frequently omits playParams.catalogId on read-back, but name/artistName
// are reliable.)
type TrackRef struct {
	Title  string
	Artist string
}

// LibraryPort reads and writes the user's target library.
//
// Idempotency for tapeIt-created playlists is tracked from what tapeIt records
// it has added (catalog ids are unreliable on read-back). For *adopting* a
// playlist the user built by hand, PlaylistTrackRefs reads the existing tracks
// by the reliable title+artist so only the genuinely missing ones are added.
type LibraryPort interface {
	// ExistingPlaylists returns a name->id map of the user's library playlists.
	ExistingPlaylists(ctx context.Context) (map[string]string, error)
	// CreatePlaylist creates an empty library playlist and returns its id.
	CreatePlaylist(ctx context.Context, name, description string) (string, error)
	// PlaylistTrackRefs returns the title+artist of each track in a playlist.
	PlaylistTrackRefs(ctx context.Context, playlistID string) ([]TrackRef, error)
	// AddTracks appends catalog songs (by id) to a library playlist.
	AddTracks(ctx context.Context, playlistID string, songIDs []string) error
}
