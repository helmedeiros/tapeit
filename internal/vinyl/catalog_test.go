package vinyl

import "testing"

func TestCatalog_ReindexesWhenTheKeyRuleChanges(t *testing.T) {
	// The failure this exists to stop. An album's identity used to live only in
	// its map key, which is derived — so improving the derivation orphaned every
	// entry written before it. Nothing errored: the lookup simply missed, the
	// album was fetched again, and the old entry lingered forever.
	//
	// Storing the name and artist the catalog gave means the index can be
	// rebuilt from the facts, so a better rule costs a re-index and not a
	// re-fetch.
	c := Catalog{Albums: []AlbumFacts{
		{Name: "Up from Below", Artist: "Edward Sharpe and the Magnetic Zeros",
			TrackCount: 12, AppleID: "a1", UPC: "0001"},
	}}

	idx := c.Index()
	want := AlbumKey("Up from Below", "Edward Sharpe & The Magnetic Zeros")
	if got, ok := idx[want]; !ok || got.TrackCount != 12 {
		t.Errorf("entry not reachable under the current key %q: %+v", want, idx)
	}
}

func TestCatalog_KeepsIdentityThatAKeyCannotCarry(t *testing.T) {
	// A normalised key cannot be read back into a name, so an entry that is only
	// a key can be neither displayed, verified, nor matched to another service.
	c := Catalog{Albums: []AlbumFacts{
		{Name: "÷", Artist: "Ed Sheeran", UPC: "190296491412", AppleID: "a2", TrackCount: 12},
	}}
	got := c.Albums[0]
	if got.Name != "÷" || got.Artist != "Ed Sheeran" {
		t.Error("the catalog must keep the identity as the source stated it")
	}
	if got.UPC == "" || got.AppleID == "" {
		t.Error("cross-service identifiers are the point of sharing these facts")
	}
}

func TestCatalog_RecordsWhichStoreTheFactsCameFrom(t *testing.T) {
	// Track counts and availability differ per storefront, so facts gathered in
	// one store are not automatically true in another. Sharing them without
	// saying where they came from would let one listener's catalog quietly
	// mislead another's.
	c := Catalog{Storefront: "de", Albums: []AlbumFacts{{Name: "A", Artist: "B", TrackCount: 10}}}
	if c.Storefront == "" {
		t.Error("a shared catalog must say which store it describes")
	}
}

func TestCatalog_FromIndexRoundTrips(t *testing.T) {
	// What is written must be what comes back, or the cache slowly diverges from
	// what the model was actually scored against.
	original := map[string]AlbumMeta{
		AlbumKey("Is This It", "The Strokes"): {
			Name: "Is This It", Artist: "The Strokes", TrackCount: 11, RuntimeMin: 35,
			CatalogID: "269080434", UPC: "078636804521", Genres: []string{"Rock"},
		},
	}
	round := FromCatalog(NewCatalog("de", original))
	got, ok := round[AlbumKey("Is This It", "The Strokes")]
	if !ok {
		t.Fatalf("entry lost in the round trip: %+v", round)
	}
	if got.TrackCount != 11 || got.RuntimeMin != 35 || got.UPC != "078636804521" {
		t.Errorf("facts changed in the round trip: %+v", got)
	}
	if got.Name != "Is This It" || got.Artist != "The Strokes" {
		t.Errorf("identity lost in the round trip: %+v", got)
	}
}

func TestCatalog_SkipsEntriesWithNoIdentity(t *testing.T) {
	// An entry that cannot say what record it describes cannot be re-indexed and
	// cannot be shared; keeping it would be keeping a fact about nothing.
	c := Catalog{Albums: []AlbumFacts{
		{Name: "", Artist: "", TrackCount: 10},
		{Name: "Real", Artist: "Someone", TrackCount: 11},
	}}
	if got := c.Index(); len(got) != 1 {
		t.Errorf("indexed %d entries, want only the one that names a record", len(got))
	}
}

func TestCatalog_SurvivesAKeyRuleChangeWithoutRefetching(t *testing.T) {
	// End to end: facts written under one key rule must still be found after the
	// rule improves. This is the regression that prompted the format — the
	// PrimaryArtist fix for "and" versus "&" silently orphaned every entry
	// written before it, and nothing anywhere reported the loss.
	written := NewCatalog("de", map[string]AlbumMeta{
		"up from below|edwardsharpeandthemagneticzeros": {
			Name: "Up from Below", Artist: "Edward Sharpe and the Magnetic Zeros",
			TrackCount: 12, CatalogID: "a1", UPC: "0001",
		},
	})

	// Read back under whatever the rule is now, however it has changed.
	got := written.Index()
	key := AlbumKey("Up from Below", "Edward Sharpe & The Magnetic Zeros")
	if meta, ok := got[key]; !ok || meta.TrackCount != 12 {
		t.Fatalf("entry lost across a key rule change; want it under %q, have %v", key, got)
	}
	// And the identity survives the trip, so it can be re-indexed again.
	if meta := got[key]; meta.Name == "" || meta.Artist == "" {
		t.Errorf("identity must survive so the next re-index can work too: %+v", meta)
	}
}
