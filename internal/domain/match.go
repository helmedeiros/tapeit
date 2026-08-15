package domain

import (
	"context"
	"errors"
)

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
	// MethodManual is a hand-pinned catalog id from the source list.
	MethodManual MatchMethod = "manual"
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
	// Album is the record the library filed this track under. Ranked listening
	// lists are track-shaped; recovering the album is what lets them be reasoned
	// about as records.
	Album string
	// CatalogID identifies the recording in the catalog, when the library knows
	// it. With it, the album can be read from the recording itself rather than
	// matched by name — exactly, and in batches.
	CatalogID string
}

// Album is catalog metadata about a record, independent of any listening.
type Album struct {
	ID     string
	Name   string
	Artist string
	// UPC is the record's barcode: the one identifier services agree on, which
	// makes album facts portable between them and shareable between people.
	UPC           string
	TrackCount    int
	RuntimeMS     int
	Genres        []string
	IsCompilation bool
}

// ErrAlbumNotFound means the catalog answered and holds no such album — a
// definitive result, safe to remember. Any other error means the question could
// not be asked (rate limiting, network), which must never be cached as an
// answer: a transient throttle would otherwise exclude the record for good.
var ErrAlbumNotFound = errors.New("album not found in catalog")

// PlayedTrack is one recording and how often the listener has played it.
type PlayedTrack struct {
	Title  string
	Artist string
	Album  string
	Count  int
}

// PlayCountPort reads how often the listener has played each track.
//
// It is a distinct port from the library because it answers a distinct
// question. A library says what someone chose to keep; a play count says what
// they actually reached for, including tracks no ranked list had room for and
// records released after the last list was drawn.
type PlayCountPort interface {
	PlayCounts(ctx context.Context) ([]PlayedTrack, error)
}

// AlbumTrack is one track as pressed on a release.
type AlbumTrack struct {
	Title      string
	DurationMS int
}

// AlbumPort resolves album metadata from the target catalog.
type AlbumPort interface {
	// SongAlbums maps catalog song ids to the id of the album containing each.
	// Exact: the album comes from the recording, not from matching its name.
	SongAlbums(ctx context.Context, songIDs []string) (map[string]string, error)
	// AlbumsByID returns metadata for catalog albums, by id.
	AlbumsByID(ctx context.Context, albumIDs []string) (map[string]Album, error)
	// Album returns the *standard* edition matching name and artist. Deluxe and
	// anniversary editions pad the track count, which understates how much of a
	// record a listener actually loves, so the smallest matching edition wins.
	Album(ctx context.Context, name, artist string) (Album, error)
	// AlbumTracks returns a release's track listing. It costs a request per
	// album, so it is asked only about a shortlist — but it answers the two
	// questions that decide a purchase: how long the record runs (one LP or
	// two), and which of the listener's loved tracks are actually on this
	// pressing rather than on some other edition.
	AlbumTracks(ctx context.Context, albumID string) ([]AlbumTrack, error)
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
