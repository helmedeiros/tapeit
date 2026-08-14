package vinyl

import (
	"math"
	"testing"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// window is the observation span used across these tests: nine yearly lists.
func window() Options { return DefaultOptions(2017, 2025) }

// album builds a plausible single-LP record so tests state only what they mean.
func album(name, artist string, loved, tracks int, years ...int) Evidence {
	return Evidence{Album: name, Artist: artist, LovedTracks: loved, TrackCount: tracks,
		RuntimeMin: 38, Years: years, RankWeight: float64(loved), MeanRankWeight: 0.35}
}

func TestScore_CoverageBeatsOneBelovedTrack(t *testing.T) {
	// The whole point of the package: a record you love most of should outrank a
	// record holding a single favourite, even when that single track ranked #1.
	deep := album("Wet Leg", "Wet Leg", 12, 12, 2022, 2024)
	hit := album("Teenage Dream", "Katy Perry", 1, 12, 2017)
	hit.RankWeight = 12 // as if its one track sat at the very top, repeatedly

	o := window()
	if Score(deep, 12, o).Score <= Score(hit, 12, o).Score {
		t.Errorf("deep album must outrank a one-track album: %.3f vs %.3f",
			Score(deep, 12, o).Score, Score(hit, 12, o).Score)
	}
}

func TestScore_FiveYearRelationshipBeatsOneYearObsession(t *testing.T) {
	// Explicit product rule: a record is permanent, so durability outweighs a
	// single year of intensity even at perfect coverage.
	durable := album("El Camino", "The Black Keys", 4, 11, 2019, 2021, 2022, 2023, 2025)
	obsession := album("Everyday Robots", "Damon Albarn", 12, 12, 2023)

	o := window()
	ds, os_ := Score(durable, 12, o).Score, Score(obsession, 12, o).Score
	if ds <= os_ {
		t.Errorf("five-year relationship must beat a one-year obsession: %.3f vs %.3f", ds, os_)
	}
}

func TestScore_RecentObsessionBeatsForgottenOne(t *testing.T) {
	// Same shape, same depth, different era: the live one is still true.
	recent := album("Recent", "A", 10, 11, 2025)
	faded := album("Faded", "B", 10, 11, 2017)

	o := window()
	if Score(recent, 10, o).Score <= Score(faded, 10, o).Score {
		t.Errorf("a current obsession must outrank an abandoned one")
	}
}

func TestScore_SoundtracksExcludedByDefault(t *testing.T) {
	s := album("The Book of Life", "Diego Luna", 8, 12, 2021)
	s.IsSoundtrack = true

	if got := Score(s, 8, window()); got.Excluded != "soundtrack" || got.Score != 0 {
		t.Errorf("soundtrack should be excluded by default, got %+v", got)
	}
	o := window()
	o.IncludeSoundtracks = true
	if got := Score(s, 8, o); got.Excluded != "" || got.Score == 0 {
		t.Errorf("--include-soundtracks should admit it, got %+v", got)
	}
}

func TestScore_SinglesAndEPsExcluded(t *testing.T) {
	single := Evidence{Album: "The Night Begins to Shine - Single", Artist: "B.E.R.",
		LovedTracks: 1, TrackCount: 1, RuntimeMin: 4, Years: []int{2018, 2019, 2020}}
	if got := Score(single, 1, window()); got.Excluded == "" {
		t.Errorf("a single is not a vinyl candidate: %+v", got)
	}
	ep := Evidence{Album: "Unlikely (Acoustic)", Artist: "Far From Alaska",
		LovedTracks: 4, TrackCount: 5, RuntimeMin: 16, Years: []int{2019}}
	if got := Score(ep, 4, window()); got.Excluded == "" {
		t.Errorf("an EP is a different purchase: %+v", got)
	}
}

func TestScore_CompilationHeavilyPenalised(t *testing.T) {
	plain := album("Brothers", "The Black Keys", 4, 11, 2019, 2021)
	comp := album("Pop Anthems", "Various", 4, 11, 2019, 2021)
	comp.IsCompilation = true

	o := window()
	got, want := Score(comp, 4, o).Score, Score(plain, 4, o).Score*compilationPenalty
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("compilation penalty not applied: %.4f want %.4f", got, want)
	}
}

func TestScore_LibraryCorroborationSurfacesQuietFavourites(t *testing.T) {
	// A yearly top-100 is truncated, so an album played steadily but never
	// obsessively barely registers. The independently saved library is the only
	// evidence that can rescue it, and must actually move the score.
	quiet := album("There Is Nothing Left to Lose", "Foo Fighters", 2, 11, 2019, 2021, 2023)
	withLib := quiet
	withLib.LibraryTracks = 11

	o := window()
	if Score(withLib, 4, o).Score <= Score(quiet, 4, o).Score {
		t.Error("saved-library depth must raise an album the ranked lists under-report")
	}
}

func TestScore_DoubleLPFlagged(t *testing.T) {
	long := album("Ofertório", "Caetano Veloso", 6, 28, 2020)
	long.RuntimeMin = 88
	if !Score(long, 6, window()).DoubleLP {
		t.Error("an 88-minute record should be flagged as a double LP")
	}
}

func TestRank_CapsOneArtistFromEatingTheList(t *testing.T) {
	ev := []Evidence{
		album("Dropout Boogie", "The Black Keys", 10, 10, 2023, 2024),
		album("El Camino", "The Black Keys", 9, 11, 2019, 2021, 2023),
		album("Brothers", "The Black Keys", 8, 11, 2019, 2021),
		album("Let's Rock", "The Black Keys", 7, 12, 2019, 2023),
		album("Is This It", "The Strokes", 9, 11, 2019, 2022),
	}
	o := window()
	o.MaxPerArtist, o.Size = 2, 5

	got := Rank(ev, o)
	n := 0
	for _, s := range got {
		if domain.PrimaryArtist(s.Artist) == "theblackkeys" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("expected the per-artist cap to hold at 2, got %d in %d results", n, len(got))
	}
}

func TestRank_ExcludedAlbumsNeverAppear(t *testing.T) {
	st := album("Trolls", "Various", 9, 14, 2018)
	st.IsSoundtrack = true
	ev := []Evidence{st, album("Is This It", "The Strokes", 9, 11, 2019, 2022)}

	for _, s := range Rank(ev, window()) {
		if s.IsSoundtrack {
			t.Error("an excluded album leaked into the ranking")
		}
	}
}

func TestRankWeight_FollowsAPowerLawNotALine(t *testing.T) {
	// A ranked year concentrates hard: first place is played many times what the
	// hundredth is. A linear curve would treat the list as nearly flat.
	if got := RankWeight(1, 100, DefaultRankAlpha); got != 1.0 {
		t.Errorf("rank 1 = %v, want 1.0", got)
	}
	top, tail := RankWeight(1, 100, DefaultRankAlpha), RankWeight(100, 100, DefaultRankAlpha)
	if ratio := top / tail; ratio < 8 || ratio > 12 {
		t.Errorf("first place should be ~10x the hundredth, got %.1fx", ratio)
	}
	// and it must decay, not step
	if !(RankWeight(2, 100, DefaultRankAlpha) > RankWeight(10, 100, DefaultRankAlpha) &&
		RankWeight(10, 100, DefaultRankAlpha) > RankWeight(50, 100, DefaultRankAlpha)) {
		t.Error("weights must decrease monotonically with rank")
	}
	if RankWeight(1, 1, DefaultRankAlpha) != 1 {
		t.Error("a single-entry list must not divide by zero")
	}
}

func TestEffectiveCoverage_CreditsTracksBelowTheCutoff(t *testing.T) {
	// Absence from a truncated list is not silence: rank 101 was played nearly as
	// much as rank 100, so raw coverage understates every record.
	raw := 0.7
	got := effectiveCoverage(raw, 0.5, DefaultCensoringCredit)
	if got <= raw {
		t.Errorf("a mostly-loved record should gain credit: %.3f vs raw %.3f", got, raw)
	}
	if got > 1 {
		t.Errorf("coverage must stay a share: %.3f", got)
	}
}

func TestEffectiveCoverage_CannotResurrectAOneHitAlbum(t *testing.T) {
	// The failure mode the correction must not have: one beloved track implying
	// the other eleven were played. Evidence about a record is what licenses the
	// inference, and one track is evidence of nothing but itself.
	oneHit := effectiveCoverage(1.0/12.0, 1.0, DefaultCensoringCredit) // its one track ranked #1
	if oneHit > 0.15 {
		t.Errorf("one hit must not imply a loved album: %.3f", oneHit)
	}
	half := effectiveCoverage(0.5, 1.0, DefaultCensoringCredit)
	if half <= oneHit {
		t.Error("the credit should grow with how much of the record already surfaced")
	}
}

func TestEffectiveCoverage_NoInferenceAtTheExtremes(t *testing.T) {
	if got := effectiveCoverage(0, 1, DefaultCensoringCredit); got != 0 {
		t.Errorf("nothing observed licenses nothing: %.3f", got)
	}
	if got := effectiveCoverage(1, 1, DefaultCensoringCredit); got != 1 {
		t.Errorf("a fully observed record has no remainder to credit: %.3f", got)
	}
	if got := effectiveCoverage(0.6, 0.5, 0); got != 0.6 {
		t.Errorf("credit 0 must restore raw coverage: %.3f", got)
	}
}

func TestScore_CensoringHelpsDeepRecordsMoreThanHitAlbums(t *testing.T) {
	// The correction should widen the gap it exists to measure, not narrow it.
	deep := album("Deep", "A", 8, 11, 2022, 2024)
	deep.MeanRankWeight = 0.4
	hit := album("Hit", "B", 1, 11, 2022, 2024)
	hit.MeanRankWeight = 1.0 // its single track sits at the very top

	on, off := window(), window()
	off.CensoringCredit = 0
	gainDeep := Score(deep, 8, on).Coverage - Score(deep, 8, off).Coverage
	gainHit := Score(hit, 8, on).Coverage - Score(hit, 8, off).Coverage
	if gainDeep <= gainHit {
		t.Errorf("censoring credit should favour records already partly loved: deep +%.3f vs hit +%.3f",
			gainDeep, gainHit)
	}
}

func TestAggregate_FoldsAppearancesAcrossYears(t *testing.T) {
	apps := []Appearance{
		{Year: 2019, Rank: 1, Size: 100, Track: "a", AlbumKey: "k", Album: "Is This It", Artist: "The Strokes"},
		{Year: 2019, Rank: 50, Size: 100, Track: "b", AlbumKey: "k", Album: "Is This It", Artist: "The Strokes"},
		{Year: 2022, Rank: 10, Size: 100, Track: "a", AlbumKey: "k", Album: "Is This It", Artist: "The Strokes"},
	}
	meta := map[string]AlbumMeta{"k": {TrackCount: 11, RuntimeMin: 35}}
	got := Aggregate(apps, meta, map[string]int{"k": 5}, window())
	if len(got) != 1 {
		t.Fatalf("want 1 album, got %d", len(got))
	}
	e := got[0]
	if e.LovedTracks != 2 {
		t.Errorf("distinct tracks = %d, want 2 (same track twice counts once)", e.LovedTracks)
	}
	if len(e.Years) != 2 {
		t.Errorf("years = %v, want 2 distinct", e.Years)
	}
	if e.LibraryTracks != 5 || e.TrackCount != 11 {
		t.Errorf("metadata not attached: %+v", e)
	}
}

func TestEvaluate_ModelBeatsNaiveBaseline(t *testing.T) {
	// A realistic listener: one deep durable record, one recurring hit single
	// that the naive baseline will certainly find, and a fresh one-hit album
	// each year that it will chase and never see again. The model should beat
	// the baseline by a finite, believable margin — an infinite lift would mean
	// the baseline was rigged to find nothing.
	var apps []Appearance
	years := []int{2019, 2020, 2021, 2022}
	for _, y := range years {
		for i := 1; i <= 8; i++ { // the deep record: many tracks, mid-chart, every year
			apps = append(apps, Appearance{Year: y, Rank: 20 + i*5, Size: 100,
				Track: "deep" + string(rune('a'+i)), AlbumKey: "deep", Album: "Deep", Artist: "Band"})
		}
		// a recurring #1 from an album the listener otherwise ignores
		apps = append(apps, Appearance{Year: y, Rank: 1, Size: 100,
			Track: "anthem", AlbumKey: "anthem", Album: "Anthem", Artist: "Other"})
		// a different one-hit wonder each year, ranked #2 and never repeated
		k := "oneoff" + string(rune('0'+y%10))
		apps = append(apps, Appearance{Year: y, Rank: 2, Size: 100,
			Track: k, AlbumKey: k, Album: k, Artist: "Ephemeral " + k})
	}
	meta := map[string]AlbumMeta{
		"deep":   {TrackCount: 10, RuntimeMin: 40},
		"anthem": {TrackCount: 12, RuntimeMin: 45},
	}
	for _, y := range years {
		meta["oneoff"+string(rune('0'+y%10))] = AlbumMeta{TrackCount: 12, RuntimeMin: 45}
	}

	o := DefaultOptions(2019, 2022)
	o.Size, o.MaxPerArtist = 2, 0
	rep := Evaluate(apps, meta, map[string]int{}, o)

	if len(rep.Years) != len(years) {
		t.Fatalf("expected %d held-out years, got %d", len(years), len(rep.Years))
	}
	for _, y := range rep.Years {
		if y.BaseHits == 0 {
			t.Fatalf("baseline must be competitive for the comparison to mean anything:\n%s", rep)
		}
	}
	if math.IsInf(rep.Lift, 1) {
		t.Fatalf("infinite lift means a rigged baseline:\n%s", rep)
	}
	if rep.Lift <= 1.0 {
		t.Errorf("model should beat the naive baseline, lift = %.2f\n%s", rep.Lift, rep)
	}
	t.Logf("model beats naive top-tracks by %.2fx\n%s", rep.Lift, rep)
}

func TestAlbumKey_FoldsEditionsAndCredits(t *testing.T) {
	// The same record reached from two services, spelled two ways.
	a := AlbumKey("Future Nostalgia (Deluxe)", "Dua Lipa")
	b := AlbumKey("Future Nostalgia", "Dua Lipa, DaBaby")
	if a != b {
		t.Errorf("editions/credits should fold to one key: %q vs %q", a, b)
	}
	if AlbumKey("El Camino", "The Black Keys") == AlbumKey("Brothers", "The Black Keys") {
		t.Error("different records must not collide")
	}
}

func TestScore_UnresolvedAlbumCannotRank(t *testing.T) {
	// Without a track count there is no coverage, and coverage is the argument
	// for owning a record. An unresolved album would otherwise score on
	// persistence and recency alone and place on a shortlist it cannot justify.
	unresolved := Evidence{Album: "Blur", Artist: "Blur", LovedTracks: 1,
		TrackCount: 0, RuntimeMin: 0, Years: []int{2018, 2019, 2021, 2023}}

	got := Score(unresolved, 4, window())
	if got.Excluded == "" || got.Score != 0 {
		t.Errorf("an album with no metadata must not rank: %+v", got)
	}
	for _, s := range Rank([]Evidence{unresolved}, window()) {
		t.Errorf("unresolved album leaked into the ranking: %+v", s)
	}
}

func TestScore_ASingleLovedTrackIsNotARecord(t *testing.T) {
	// The premise of the package, applied to itself: a record represented by one
	// track in years of listening is a single you liked, not an album you live
	// with. Persistence and recency could otherwise carry it onto a shortlist on
	// the strength of that one song — which is the hit-chasing this exists to
	// avoid, arriving by a different route.
	oneTrack := album("One By One", "Foo Fighters", 1, 17, 2019, 2023)

	got := Score(oneTrack, 4, window())
	if got.Excluded == "" || got.Score != 0 {
		t.Errorf("one loved track cannot justify a record: %+v", got)
	}
}

func TestScore_LibraryDepthRedeemsAThinListeningRecord(t *testing.T) {
	// The exception that must survive: the ranked lists truncate at 100, so a
	// record played steadily but never obsessively barely appears in them. When
	// the listener has separately saved most of it, that is the stronger witness
	// and the record belongs on the list.
	quiet := album("There Is Nothing Left to Lose", "Foo Fighters", 1, 11, 2019, 2023)
	quiet.LibraryTracks = 11

	if got := Score(quiet, 4, window()); got.Excluded != "" || got.Score == 0 {
		t.Errorf("a record saved in full should not be excluded for thin chart presence: %+v", got)
	}
}
