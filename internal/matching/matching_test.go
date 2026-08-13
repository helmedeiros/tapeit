package matching

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/helmedeiros/tapeit/internal/domain"
)

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"Hello, World!":       "hello world",
		"  Multiple   Spaces": "multiple spaces",
		"AC/DC — Thunder":     "acdc thunder",
		"":                    "",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestKey(t *testing.T) {
	withISRC := domain.Track{Title: "X", Artists: []string{"Y"}, ISRC: "usabc1234567"}
	if got := Key(withISRC); got != "isrc:USABC1234567" {
		t.Errorf("Key with ISRC = %q", got)
	}
	noISRC := domain.Track{Title: "Hello!", Artists: []string{"The Band"}}
	if got := Key(noISRC); got != "tt:hello|the band" {
		t.Errorf("Key without ISRC = %q", got)
	}
}

func TestPickBest_ClosestDuration(t *testing.T) {
	track := domain.Track{DurationMS: 200000}
	cands := []domain.CatalogSong{
		{ID: "long", DurationMS: 260000},
		{ID: "close", DurationMS: 201000},
		{ID: "short", DurationMS: 120000},
	}
	best, ok := pickBest(track, cands)
	if !ok || best.ID != "close" {
		t.Errorf("pickBest = %+v, ok=%v; want close", best, ok)
	}
	if _, ok := pickBest(track, nil); ok {
		t.Errorf("pickBest(nil) should be false")
	}
}

func TestCleanTitle(t *testing.T) {
	cases := map[string]string{
		"Rebel Rebel - 2016 Remaster":        "Rebel Rebel",
		"I Sat By The Ocean (Live Acoustic)": "I Sat By The Ocean",
		"Plain Title":                        "Plain Title",
		"Daisy - Spotify Singles":            "Daisy",
	}
	for in, want := range cases {
		if got := cleanTitle(in); got != want {
			t.Errorf("cleanTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTitleScore(t *testing.T) {
	if titleScore("Song", "Song") != 1.0 {
		t.Error("exact title should score 1.0")
	}
	if titleScore("Rebel Rebel - 2016 Remaster", "Rebel Rebel") != 0.95 {
		t.Error("suffix-stripped title should score 0.95")
	}
	if titleScore("Totally Different", "Nope") != 0.0 {
		t.Error("unrelated title should score 0.0")
	}
}

func TestSuffixedSearchMatch(t *testing.T) {
	fc := fakeCatalog{search: []domain.CatalogSong{
		{ID: "remaster-hit", Title: "Rebel Rebel", Artist: "David Bowie", DurationMS: 270000},
	}}
	track := domain.Track{Title: "Rebel Rebel - 2016 Remaster", Artists: []string{"David Bowie"}, DurationMS: 270500}
	got, err := matchAll(fc, []domain.Track{track})
	if err != nil {
		t.Fatal(err)
	}
	if !got[0].Matched() || got[0].AppleID != "remaster-hit" {
		t.Errorf("expected suffixed title to match base recording, got %+v", got[0])
	}
}

func TestPickScored(t *testing.T) {
	track := domain.Track{Title: "Song Name", Artists: []string{"Artist"}, DurationMS: 200000}
	good := domain.CatalogSong{ID: "g", Title: "Song Name", Artist: "Artist", DurationMS: 200500}
	if _, conf := pickScored(track, []domain.CatalogSong{good}); conf != domain.ConfHigh {
		t.Errorf("expected high confidence, got %s", conf)
	}
	bad := domain.CatalogSong{ID: "b", Title: "Totally Different", Artist: "Someone", DurationMS: 10}
	if _, conf := pickScored(track, []domain.CatalogSong{bad}); conf != domain.ConfNone {
		t.Errorf("expected none, got %s", conf)
	}
}

// fakeCatalog implements domain.CatalogPort for the service test.
type fakeCatalog struct {
	byISRC map[string][]domain.CatalogSong
	search []domain.CatalogSong
}

func (f fakeCatalog) SongsByISRC(_ context.Context, isrcs []string) (map[string][]domain.CatalogSong, error) {
	out := map[string][]domain.CatalogSong{}
	for _, code := range isrcs {
		if v, ok := f.byISRC[code]; ok {
			out[code] = v
		}
	}
	return out, nil
}

func (f fakeCatalog) SearchSongs(_ context.Context, _ string, _ int) ([]domain.CatalogSong, error) {
	return f.search, nil
}

func TestService_Match(t *testing.T) {
	fc := fakeCatalog{
		byISRC: map[string][]domain.CatalogSong{
			"USAAA0000001": {{ID: "apple-1", DurationMS: 180000}},
		},
		search: []domain.CatalogSong{{ID: "apple-2", Title: "Fallback", Artist: "Band", DurationMS: 200000}},
	}
	tracks := []domain.Track{
		{Title: "Has ISRC", Artists: []string{"A"}, ISRC: "USAAA0000001", DurationMS: 180000},
		{Title: "Fallback", Artists: []string{"Band"}, DurationMS: 200000},  // no ISRC -> search
		{Title: "Missing", Artists: []string{"None"}, ISRC: "USZZZ9999999"}, // ISRC absent -> search -> none
	}

	got, err := matchAll(fc, tracks)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Method != domain.MethodISRC || got[0].AppleID != "apple-1" {
		t.Errorf("track 0: %+v", got[0])
	}
	if got[1].Method != domain.MethodSearch || got[1].AppleID != "apple-2" {
		t.Errorf("track 1: %+v", got[1])
	}
	if got[2].Matched() {
		t.Errorf("track 2 should be unmatched: %+v", got[2])
	}
}

func matchAll(c domain.CatalogPort, tracks []domain.Track) ([]domain.Match, error) {
	return New(c, nil).Match(context.Background(), tracks)
}

// spyCatalog is a CatalogPort that records how often it was consulted, so the
// chaining tests can assert a later catalog is only asked when needed.
type spyCatalog struct {
	songs     []domain.CatalogSong
	byISRC    map[string][]domain.CatalogSong
	err       error
	searches  int
	isrcCalls int
}

func (s *spyCatalog) SearchSongs(_ context.Context, _ string, _ int) ([]domain.CatalogSong, error) {
	s.searches++
	if s.err != nil {
		return nil, s.err
	}
	return s.songs, nil
}

func (s *spyCatalog) SongsByISRC(_ context.Context, isrcs []string) (map[string][]domain.CatalogSong, error) {
	s.isrcCalls++
	if s.err != nil {
		return nil, s.err
	}
	out := map[string][]domain.CatalogSong{}
	for _, code := range isrcs {
		if v, ok := s.byISRC[strings.ToUpper(code)]; ok {
			out[strings.ToUpper(code)] = v
		}
	}
	return out, nil
}

// gorillazTrack is the real-world case that motivated catalog chaining: the
// iTunes Search index has no Song Machine entry, so it answers with plausible
// but wrong songs while the amp-api catalog has the real recording.
func gorillazTrack() domain.Track {
	return domain.Track{Title: "Pac-Man (feat. ScHoolboy Q)", Artists: []string{"Gorillaz"}, DurationMS: 192000}
}

func TestChained_FallsBackWhenPrimaryHasNothingAcceptable(t *testing.T) {
	primary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "wrong", Title: "Pac-Man (Main Theme)", Artist: "8-Bit Arcade", DurationMS: 30000},
	}}
	secondary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "right", Title: "Pac-Man (feat. ScHoolboy Q)", Artist: "Gorillaz", DurationMS: 192500},
	}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "right" || got[0].Confidence != domain.ConfHigh {
		t.Errorf("want secondary high match, got %+v", got[0])
	}
	if secondary.searches != 1 {
		t.Errorf("secondary searched %d times, want 1", secondary.searches)
	}
}

func TestChained_StopsAtFirstHighConfidence(t *testing.T) {
	primary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "right", Title: "Pac-Man (feat. ScHoolboy Q)", Artist: "Gorillaz", DurationMS: 192500},
	}}
	secondary := &spyCatalog{songs: []domain.CatalogSong{{ID: "other", Title: "Irrelevant", Artist: "Nobody", DurationMS: 10000}}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "right" {
		t.Errorf("want primary match, got %+v", got[0])
	}
	if secondary.searches != 0 {
		t.Errorf("secondary should not be consulted after a high-confidence hit, got %d calls", secondary.searches)
	}
}

func TestChained_UpgradesLowConfidenceFromLaterCatalog(t *testing.T) {
	// Primary can only offer a prefix-title match (low); secondary has the exact
	// recording, so the better answer must win rather than the first one.
	primary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "weak", Title: "Pac-Man", Artist: "Gorillaz", DurationMS: 250000},
	}}
	secondary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "right", Title: "Pac-Man (feat. ScHoolboy Q)", Artist: "Gorillaz", DurationMS: 192500},
	}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "right" || got[0].Confidence != domain.ConfHigh {
		t.Errorf("want upgraded high match, got %+v", got[0])
	}
}

func TestChained_KeepsBestLowWhenNoCatalogHasHigh(t *testing.T) {
	primary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "weak", Title: "Pac-Man", Artist: "Gorillaz", DurationMS: 250000},
	}}
	secondary := &spyCatalog{songs: []domain.CatalogSong{{ID: "junk", Title: "Nope", Artist: "Nobody", DurationMS: 10000}}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "weak" || got[0].Confidence != domain.ConfLow {
		t.Errorf("want the primary's low match retained, got %+v", got[0])
	}
}

func TestChained_UnmatchedWhenNoCatalogHasIt(t *testing.T) {
	primary := &spyCatalog{songs: []domain.CatalogSong{{ID: "a", Title: "Nope", Artist: "Nobody", DurationMS: 10000}}}
	secondary := &spyCatalog{songs: []domain.CatalogSong{{ID: "b", Title: "Also No", Artist: "Nobody", DurationMS: 11000}}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Matched() {
		t.Errorf("want unmatched, got %+v", got[0])
	}
}

func TestChained_SurvivesPrimaryError(t *testing.T) {
	// A rate-limited primary must not cost us a track the secondary can resolve.
	primary := &spyCatalog{err: errors.New("429 rate limited")}
	secondary := &spyCatalog{songs: []domain.CatalogSong{
		{ID: "right", Title: "Pac-Man (feat. ScHoolboy Q)", Artist: "Gorillaz", DurationMS: 192500},
	}}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "right" {
		t.Errorf("want secondary match despite primary error, got %+v", got[0])
	}
}

func TestChained_ErrorsOnlyWhenEveryCatalogFails(t *testing.T) {
	primary := &spyCatalog{err: errors.New("boom")}
	secondary := &spyCatalog{err: errors.New("boom")}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{gorillazTrack()})
	if err != nil {
		t.Fatal(err)
	}
	// A failed search is recorded as unmatched (not fatal) so one bad lookup
	// cannot discard the rest of the run.
	if got[0].Matched() || got[0].Note == "" {
		t.Errorf("want unmatched with a note, got %+v", got[0])
	}
}

func TestChained_ISRCFallsBackToLaterCatalog(t *testing.T) {
	primary := &spyCatalog{} // no ISRC index at all, like the iTunes Search API
	secondary := &spyCatalog{byISRC: map[string][]domain.CatalogSong{
		"USAAA0000001": {{ID: "apple-1", DurationMS: 180000}},
	}}
	track := domain.Track{Title: "Has ISRC", Artists: []string{"A"}, ISRC: "usaaa0000001", DurationMS: 180000}

	got, err := NewChained(nil, primary, secondary).Match(context.Background(), []domain.Track{track})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AppleID != "apple-1" || got[0].Method != domain.MethodISRC {
		t.Errorf("want ISRC match from secondary, got %+v", got[0])
	}
}
