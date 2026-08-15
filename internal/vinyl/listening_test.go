package vinyl

import (
	"math"
	"testing"
)

func TestEstimatedPlays_AnchorsTheChartCurveToRealCounts(t *testing.T) {
	// A yearly chart is a ranking, not a measurement — but the listener knows
	// roughly what the top of one is worth: the songs there were played 80-odd
	// times that year. That turns a rank into an estimate of listening, which is
	// the quantity the shortlist is actually trying to maximise.
	o := DefaultOptions(2017, 2025)
	if got := EstimatedPlaysAt(1, 100, o); math.Abs(got-TopPlaysPerYear) > 1 {
		t.Errorf("rank 1 = %.0f plays, want about %d", got, TopPlaysPerYear)
	}
	top, tail := EstimatedPlaysAt(1, 100, o), EstimatedPlaysAt(100, 100, o)
	if ratio := top / tail; ratio < 8 || ratio > 12 {
		t.Errorf("top should be worth roughly ten times the tail, got %.1fx", ratio)
	}
	if EstimatedPlaysAt(10, 100, o) <= EstimatedPlaysAt(50, 100, o) {
		t.Error("estimated plays must fall with rank")
	}
}

func TestAggregate_CountsChartedAndActualListeningTogether(t *testing.T) {
	// The two sources measure the same thing in different units: a chart rank
	// implies a number of plays, and the play history states one outright.
	// Intensity read only the charts, so a record played constantly but never
	// charted — anything newer than the last list — registered as unlistened.
	o := DefaultOptions(2017, 2025)
	charted := Aggregate(
		[]Appearance{{Year: 2024, Rank: 1, Size: 100, Track: "a", AlbumKey: "k", Album: "A", Artist: "X"}},
		nil, map[string]AlbumMeta{"k": {TrackCount: 10}}, map[string]int{}, o)
	played := Aggregate(nil,
		[]Play{
			{Track: "a", AlbumKey: "k2", Album: "B", Artist: "Y", Count: 40},
			{Track: "b", AlbumKey: "k2", Album: "B", Artist: "Y", Count: 40},
		},
		map[string]AlbumMeta{"k2": {TrackCount: 10}}, map[string]int{}, o)

	if charted[0].EstimatedPlays <= 0 {
		t.Error("a top-ranked chart entry implies substantial listening")
	}
	if played[0].EstimatedPlays < 80 {
		t.Errorf("80 counted plays should register as at least that: %.0f", played[0].EstimatedPlays)
	}
}

func TestScore_NonStopListeningIsNotLeftBehindByBreadth(t *testing.T) {
	// The balance the listener asked for. A record with a handful of tracks
	// played relentlessly must stay competitive with one whose tracks are merely
	// numerous, or the shortlist optimises for albums that are broadly pleasant
	// over records that were actually lived in.
	o := DefaultOptions(2017, 2025)
	broad := Evidence{Album: "Broad", Artist: "A", LovedTracks: 8, TrackCount: 11,
		RuntimeMin: 40, Years: []int{2022, 2023}, EstimatedPlays: 60, MeanRankWeight: 0.2}
	deep := Evidence{Album: "Relentless", Artist: "B", LovedTracks: 3, TrackCount: 11,
		RuntimeMin: 40, Years: []int{2022, 2023}, EstimatedPlays: 240, MeanRankWeight: 0.9}

	bs, ds := Score(broad, 240, o), Score(deep, 240, o)
	if ds.Score <= bs.Score*0.8 {
		t.Errorf("relentless listening should stay competitive: %.3f vs broad %.3f", ds.Score, bs.Score)
	}
	if ds.Intensity <= bs.Intensity {
		t.Errorf("intensity should favour the record actually played more: %.3f vs %.3f",
			ds.Intensity, bs.Intensity)
	}
}

func TestScore_ManyYearsStillOutranksASingleHeavyYear(t *testing.T) {
	// The listener's first rule, unchanged: a record heard across many years sits
	// above all, whatever a single year's intensity says.
	o := DefaultOptions(2017, 2025)
	durable := Evidence{Album: "Durable", Artist: "A", LovedTracks: 4, TrackCount: 11,
		RuntimeMin: 38, Years: []int{2019, 2021, 2022, 2023, 2025}, EstimatedPlays: 120, MeanRankWeight: 0.3}
	oneHeavyYear := Evidence{Album: "One Year", Artist: "B", LovedTracks: 11, TrackCount: 11,
		RuntimeMin: 38, Years: []int{2023}, EstimatedPlays: 300, MeanRankWeight: 0.9}

	if Score(durable, 300, o).Score <= Score(oneHeavyYear, 300, o).Score {
		t.Errorf("five years must outrank one heavy year: %.3f vs %.3f",
			Score(durable, 300, o).Score, Score(oneHeavyYear, 300, o).Score)
	}
}

func TestScore_IntensityIsCompressedSoOneRecordCannotFlattenTheField(t *testing.T) {
	// One album played enormously more than the rest would otherwise drive every
	// other record's intensity to nearly zero, silently turning a five-signal
	// model into a four-signal one.
	o := DefaultOptions(2017, 2025)
	modest := Evidence{Album: "Modest", Artist: "A", LovedTracks: 6, TrackCount: 11,
		RuntimeMin: 38, Years: []int{2022, 2023}, EstimatedPlays: 100}
	if got := Score(modest, 2000, o).Intensity; got < 0.15 {
		t.Errorf("intensity = %.3f — a merely well-played record has been flattened", got)
	}
}
