package vinyl

import (
	"sort"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// A record's facts were previously held in a map whose key was the only place
// its identity lived — and that key is derived. Improving the derivation
// therefore orphaned every entry written before it: the lookup missed, the album
// was fetched again, and the stale entry lingered unreachable. Nothing reported
// any of it, because a cache miss looks exactly like a cache that was empty.
//
// Keeping the name and artist the catalog gave, alongside the identifiers other
// services share, fixes that and makes the file worth passing to someone else:
// an entry can be re-indexed, displayed, verified, and matched to another
// service without re-deriving anything.

// CatalogVersion is the shape of the stored file. It changes when a reader can
// no longer trust what an older file means.
const CatalogVersion = 1

// AlbumFacts is what is known about one record, stated rather than derived.
type AlbumFacts struct {
	// Name and Artist as the catalog gives them. These are the identity: the
	// index key is rebuilt from these, never the other way round.
	Name   string `json:"name"`
	Artist string `json:"artist"`
	// UPC is the record's barcode, the identifier services agree on and so the
	// one that makes these facts portable to a listener on another service.
	UPC string `json:"upc,omitempty"`
	// AppleID is the catalog id in the storefront these facts came from.
	AppleID string `json:"apple_id,omitempty"`

	TrackCount    int      `json:"track_count"`
	RuntimeMin    int      `json:"runtime_min,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	IsCompilation bool     `json:"is_compilation,omitempty"`
	IsSoundtrack  bool     `json:"is_soundtrack,omitempty"`
}

// Catalog is a shareable set of album facts.
type Catalog struct {
	Version int `json:"version"`
	// Storefront says which store these facts describe. Track counts, runtimes
	// and availability differ between them, so facts gathered in one store are
	// not automatically true in another — and a shared file that did not say
	// where it came from could quietly mislead the next reader.
	Storefront string       `json:"storefront"`
	Albums     []AlbumFacts `json:"albums"`
}

// Index rebuilds the lookup map under the current key rule. Because identity is
// stored rather than inferred, a better rule costs a re-index and not a refetch.
func (c Catalog) Index() map[string]AlbumMeta {
	out := make(map[string]AlbumMeta, len(c.Albums))
	for _, a := range c.Albums {
		if a.Name == "" && a.Artist == "" {
			// An entry that cannot say which record it describes cannot be
			// re-indexed or shared; it is a fact about nothing.
			continue
		}
		out[domain.AlbumKey(a.Name, a.Artist)] = AlbumMeta{
			Name: a.Name, Artist: a.Artist, UPC: a.UPC, CatalogID: a.AppleID,
			TrackCount: a.TrackCount, RuntimeMin: a.RuntimeMin, Genres: a.Genres,
			IsCompilation: a.IsCompilation, IsSoundtrack: a.IsSoundtrack,
		}
	}
	return out
}

// NewCatalog turns the working map into the shareable form.
func NewCatalog(storefront string, meta map[string]AlbumMeta) Catalog {
	c := Catalog{Version: CatalogVersion, Storefront: storefront, Albums: make([]AlbumFacts, 0, len(meta))}
	for _, m := range meta {
		if m.Name == "" && m.Artist == "" {
			continue
		}
		c.Albums = append(c.Albums, AlbumFacts{
			Name: m.Name, Artist: m.Artist, UPC: m.UPC, AppleID: m.CatalogID,
			TrackCount: m.TrackCount, RuntimeMin: m.RuntimeMin, Genres: m.Genres,
			IsCompilation: m.IsCompilation, IsSoundtrack: m.IsSoundtrack,
		})
	}
	// Sorted so the file is stable between runs and diffs are readable — a
	// catalog meant to be shared and versioned should not churn on every write.
	sort.Slice(c.Albums, func(i, j int) bool {
		if c.Albums[i].Artist != c.Albums[j].Artist {
			return c.Albums[i].Artist < c.Albums[j].Artist
		}
		return c.Albums[i].Name < c.Albums[j].Name
	})
	return c
}

// FromCatalog is Index, named for the direction callers read it in.
func FromCatalog(c Catalog) map[string]AlbumMeta { return c.Index() }
