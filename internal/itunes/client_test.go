package itunes

import "testing"

func boolPtr(b bool) *bool { return &b }

func TestResultToDomain_DropsUnstreamable(t *testing.T) {
	// The real case: "Trying Your Luck" id 616296935 matches perfectly on title
	// and artist but is not streamable, so Apple accepts it and then drops it.
	dead := result{TrackID: 616296935, TrackName: "Trying Your Luck", ArtistName: "The Strokes", IsStreamable: boolPtr(false)}
	if _, ok := dead.toDomain(); ok {
		t.Error("unstreamable result must be rejected")
	}

	live := result{TrackID: 269080636, TrackName: "Trying Your Luck", ArtistName: "The Strokes", IsStreamable: boolPtr(true)}
	got, ok := live.toDomain()
	if !ok || got.ID != "269080636" {
		t.Errorf("streamable result must be kept, got %+v ok=%v", got, ok)
	}
}

func TestResultToDomain_KeepsWhenFieldAbsent(t *testing.T) {
	// Absent means unknown; dropping on absence would empty every result set.
	unknown := result{TrackID: 1, TrackName: "X", ArtistName: "Y"}
	if _, ok := unknown.toDomain(); !ok {
		t.Error("absent isStreamable must be treated as playable")
	}
}
