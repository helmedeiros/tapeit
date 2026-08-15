package vinyl

import "testing"

func TestAggregate_CountsAPlayedTrackAsLovedEvenIfItNeverCharted(t *testing.T) {
	// The Franz Ferdinand case. A yearly chart holds 100 songs, so a record whose
	// listener plays it end to end still surfaces one or two tracks — here the
	// same single every year. The play history is not truncated, and says plainly
	// that eleven tracks of the record get played. Both are evidence about the
	// same thing, so coverage should see their union.
	apps := []Appearance{
		{Year: 2019, Rank: 22, Size: 100, Track: "take me out", AlbumKey: "ff", Album: "Franz Ferdinand", Artist: "Franz Ferdinand"},
		{Year: 2022, Rank: 90, Size: 100, Track: "take me out", AlbumKey: "ff", Album: "Franz Ferdinand", Artist: "Franz Ferdinand"},
	}
	plays := []Play{
		{Track: "take me out", AlbumKey: "ff", Album: "Franz Ferdinand", Artist: "Franz Ferdinand", Count: 6},
		{Track: "jacqueline", AlbumKey: "ff", Album: "Franz Ferdinand", Artist: "Franz Ferdinand", Count: 3},
		{Track: "michael", AlbumKey: "ff", Album: "Franz Ferdinand", Artist: "Franz Ferdinand", Count: 1},
	}
	meta := map[string]AlbumMeta{"ff": {TrackCount: 11, RuntimeMin: 38}}

	got := Aggregate(apps, plays, meta, map[string]int{}, DefaultOptions(2017, 2025))
	if len(got) != 1 {
		t.Fatalf("want one album, got %d", len(got))
	}
	if got[0].LovedTracks != 3 {
		t.Errorf("loved tracks = %d, want 3 (one charted, two more played)", got[0].LovedTracks)
	}
	if got[0].PlayedTracks != 3 || got[0].Plays != 10 {
		t.Errorf("play evidence not recorded: %+v", got[0])
	}
}

func TestAggregate_SurfacesAlbumsThatOnlyEverPlayed(t *testing.T) {
	// The moisturizer case. A record released after the last charted year has no
	// chart presence at all, so it could never become a candidate however much
	// its owner plays it. Play history is the only witness such a record has.
	plays := []Play{
		{Track: "catch these fists", AlbumKey: "moist", Album: "moisturizer", Artist: "Wet Leg", Count: 4},
		{Track: "mangetout", AlbumKey: "moist", Album: "moisturizer", Artist: "Wet Leg", Count: 3},
	}
	meta := map[string]AlbumMeta{"moist": {TrackCount: 12, RuntimeMin: 38}}

	got := Aggregate(nil, plays, meta, map[string]int{}, DefaultOptions(2017, 2025))
	if len(got) != 1 || got[0].Album != "moisturizer" {
		t.Fatalf("a played-but-never-charted record must still be seen: %+v", got)
	}
	if got[0].LovedTracks != 2 {
		t.Errorf("loved tracks = %d, want 2", got[0].LovedTracks)
	}
}

func TestAggregate_DoesNotDoubleCountATrackBothChartedAndPlayed(t *testing.T) {
	apps := []Appearance{
		{Year: 2024, Rank: 5, Size: 100, Track: "wet dream", AlbumKey: "wl", Album: "Wet Leg", Artist: "Wet Leg"},
	}
	plays := []Play{
		{Track: "wet dream", AlbumKey: "wl", Album: "Wet Leg", Artist: "Wet Leg", Count: 3},
	}
	got := Aggregate(apps, plays, map[string]AlbumMeta{"wl": {TrackCount: 12}}, map[string]int{},
		DefaultOptions(2017, 2025))
	if got[0].LovedTracks != 1 {
		t.Errorf("loved tracks = %d, want 1 — the same track from two sources is one track", got[0].LovedTracks)
	}
}

func TestAggregate_PlayEvidenceDoesNotInventChartYears(t *testing.T) {
	// Play history covers a short window and says nothing about durability. It
	// must not be mistaken for a year of chart presence, or a fortnight's
	// obsession would read as a relationship.
	plays := []Play{{Track: "a", AlbumKey: "k", Album: "New", Artist: "X", Count: 9}}
	got := Aggregate(nil, plays, map[string]AlbumMeta{"k": {TrackCount: 10}}, map[string]int{},
		DefaultOptions(2017, 2025))

	if len(got[0].Years) != 0 {
		t.Errorf("plays must not create chart years: %v", got[0].Years)
	}
	if Score(got[0], 10, DefaultOptions(2017, 2025)).Persistence != 0 {
		t.Error("a record known only from recent plays has shown no durability")
	}
}

func TestScore_PlayedRecordStillNeedsMoreThanOneLovedTrack(t *testing.T) {
	// The single-track guard must apply to play evidence too, or exploring an
	// album once would qualify it.
	e := Evidence{Album: "Explored", Artist: "X", LovedTracks: 1, PlayedTracks: 1,
		TrackCount: 12, RuntimeMin: 40, Years: nil}
	if got := Score(e, 4, DefaultOptions(2017, 2025)); got.Excluded == "" {
		t.Errorf("one track is one track whatever the source: %+v", got)
	}
}

func TestScore_ARecordKnownOnlyFromPlaysIsCurrentByDefinition(t *testing.T) {
	// Play counts cover a recent window and nothing else, so a record that
	// appears only in them is being listened to now. Reading recency from chart
	// years alone scored it zero — the least current possible — which is exactly
	// backwards, and it is how a record released after the last charted year is
	// penalised for being new twice over.
	e := Evidence{Album: "moisturizer", Artist: "Wet Leg", LovedTracks: 10, PlayedTracks: 10,
		Plays: 16, TrackCount: 12, RuntimeMin: 38}

	got := Score(e, 16, DefaultOptions(2017, 2025))
	if got.Recency != 1 {
		t.Errorf("recency = %.2f, want 1 — the plays are current by construction", got.Recency)
	}
	// It must still show no durability: plays say nothing about how long.
	if got.Persistence != 0 {
		t.Errorf("persistence = %.2f, want 0 — a new record has shown none", got.Persistence)
	}
}

func TestScore_ChartYearsStillDecideRecencyWhenPresent(t *testing.T) {
	// Play evidence must not paper over an abandoned record: one charted long
	// ago and barely played now should stay stale.
	old := Evidence{Album: "Faded", Artist: "A", LovedTracks: 4, TrackCount: 11,
		RuntimeMin: 38, Years: []int{2017}}
	recent := Evidence{Album: "Current", Artist: "B", LovedTracks: 4, TrackCount: 11,
		RuntimeMin: 38, Years: []int{2025}}

	o := DefaultOptions(2017, 2025)
	if Score(old, 4, o).Recency >= Score(recent, 4, o).Recency {
		t.Error("a record last charted in 2017 is not as current as one charted in 2025")
	}
}
