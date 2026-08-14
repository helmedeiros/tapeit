// Package vinyl ranks albums worth owning on vinyl from evidence about how a
// listener actually used them. It is a pure application service: no I/O, no
// provider types, so the scoring can be reasoned about and tested on its own.
//
// The problem it solves is a mismatch of units. A yearly "top songs" list ranks
// *tracks*; a record is an *album*. Ranking albums by "contains one beloved
// track" recommends singles and various-artist soundtracks, because a #1 track
// says nothing about the other nine songs on the side you would have to sit
// through. Coverage — how much of the record you actually love — is the signal
// that distinguishes a record from a hit.
package vinyl

import (
	"math"
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// Evidence is what is known about one album, gathered from ranked listening
// lists plus an independent saved library. It carries no provider types.
type Evidence struct {
	Album  string
	Artist string

	// LovedTracks is how many distinct tracks of this album appear anywhere in
	// the ranked lists, across every edition. Together with TrackCount this
	// gives coverage before the pressing has been checked.
	LovedTracks int
	// LovedTitles are those tracks by name, so a caller can ask which of them a
	// particular pressing actually holds.
	LovedTitles []string
	// PressingTracks is how many loved tracks are on the pressing being judged,
	// and PressingChecked says whether that was established. The two are
	// separate because "none of them are on this edition" is a real answer and
	// must not be mistaken for "not looked at yet" — a listener's loved tracks
	// are gathered across editions, but they buy one record.
	PressingTracks  int
	PressingChecked bool
	// Years are the distinct list years the album appeared in.
	Years []int
	// RankWeight sums each appearance's rank weight (top of a list counts more).
	RankWeight float64
	// MeanRankWeight is the average strength of this album's observed tracks. It
	// says how high in the lists the record sits, which is what licenses any
	// inference about the tracks that never surfaced.
	MeanRankWeight float64
	// LibraryTracks is how many distinct tracks of this album are in the
	// listener's separately saved library — an independent corroboration of
	// devotion that ranked lists cannot provide, since they are truncated.
	LibraryTracks int

	// TrackCount is the *standard* edition's track count. Deluxe and anniversary
	// editions pad the denominator and understate coverage, so callers should
	// resolve the smallest edition.
	TrackCount int
	// RuntimeMin is the standard edition's runtime; beyond ~70 minutes a record
	// becomes a double LP, which is a different purchase.
	RuntimeMin int

	IsCompilation bool
	IsSoundtrack  bool
	// CatalogID lets a caller fill in RuntimeMin later, for the shortlist only.
	CatalogID string
}

// Weights tune the relative pull of each signal. They should sum to 1.
type Weights struct {
	Coverage      float64
	Persistence   float64
	Recency       float64
	Intensity     float64
	Corroboration float64
}

// DefaultWeights favours persistence over coverage: a record is a permanent
// object, so a listener's five-year relationship with it is stronger evidence
// than one year of obsession. Recency then separates a live obsession from a
// faded one — both span a single year, but only one is still true.
//
// Intensity is deliberately small. It measures how high the tracks ranked,
// which an album with high coverage will score well on almost by construction —
// weighting both heavily counts the same fact twice and lets a single year of
// obsession outrank a decade-long relationship.
func DefaultWeights() Weights {
	return Weights{Coverage: 0.25, Persistence: 0.35, Recency: 0.15, Intensity: 0.10, Corroboration: 0.15}
}

// DoubleLPMinutes is where a record stops fitting on one disc, which changes
// both what it costs and how often the listener gets up to turn it over.
const DoubleLPMinutes = 70

// compilationPenalty keeps hits collections in the running but far down: they
// are real listening, and a poor thing to own as a record.
const compilationPenalty = 0.35

// Options tune a ranking run.
type Options struct {
	Size         int // how many albums to return (0 = all)
	MaxPerArtist int // 0 = unlimited; keeps one artist from eating the list
	// IncludeSoundtracks admits film and children's soundtracks. Off by default:
	// they are frequently household listening rather than the focused solo
	// listening a record is bought for.
	IncludeSoundtracks bool
	// MinTracks and MinRuntimeMin exclude singles and EPs, which are a different
	// purchase from an album.
	MinTracks     int
	MinRuntimeMin int
	// MinLovedTracks is how many of a record's tracks must have reached the
	// ranked lists before it counts as a record the listener lives with rather
	// than a single they liked. A listener who saved most of the album
	// separately satisfies this by that route instead.
	MinLovedTracks int
	// FirstYear and LastYear bound the observation window; LastYear also anchors
	// recency, so "recent" means recent relative to the data, not to the clock.
	FirstYear, LastYear int
	// RankAlpha shapes the assumed play distribution across a ranked list (see
	// RankWeight). Higher means a steeper drop from first place to last.
	RankAlpha float64
	// CensoringCredit is how much of an album's unheard remainder to credit as
	// "played, just below the cutoff" (see Score). 0 disables the correction and
	// restores the naive assumption that absence means silence.
	CensoringCredit float64
	Weights         Weights
}

// DefaultOptions returns the recommended settings for a vinyl shortlist.
func DefaultOptions(firstYear, lastYear int) Options {
	return Options{
		Size: 20, MaxPerArtist: 2,
		MinTracks: 7, MinRuntimeMin: 25, MinLovedTracks: 2,
		FirstYear: firstYear, LastYear: lastYear,
		RankAlpha: DefaultRankAlpha, CensoringCredit: DefaultCensoringCredit,
		Weights: DefaultWeights(),
	}
}

// Scored is an album with its score and the reasoning behind it, so a
// recommendation can justify itself rather than assert a number.
type Scored struct {
	Evidence
	Score float64
	// Observed is the raw share of the record seen in the lists; Coverage is that
	// share after correcting for the lists being truncated.
	Observed      float64
	Coverage      float64
	Persistence   float64
	Recency       float64
	Intensity     float64
	Corroboration float64
	DoubleLP      bool
	Excluded      string // non-empty when filtered out, saying why
}

// span is how many years the observation window covers (at least 1).
func (o Options) span() float64 {
	if o.LastYear <= o.FirstYear {
		return 1
	}
	return float64(o.LastYear - o.FirstYear + 1)
}

// exclusion reports why an album cannot be a vinyl candidate, or "".
func (o Options) exclusion(e Evidence) string {
	switch {
	case e.TrackCount <= 0:
		// Without a track count there is no coverage, and coverage is the whole
		// argument for a record. Scoring it anyway would let an unresolved album
		// place on persistence and recency alone — evidence about a listener's
		// habit, but none about whether the record is worth owning.
		return "no album metadata"
	case e.TrackCount < o.MinTracks:
		return "single or EP"
	case e.RuntimeMin > 0 && e.RuntimeMin < o.MinRuntimeMin:
		return "too short for an LP"
	case e.IsSoundtrack && !o.IncludeSoundtracks:
		return "soundtrack"
	case e.thinlyHeard(o):
		// One track in years of listening is a single the listener liked, not a
		// record they live with. Without this, persistence and recency can carry
		// such a record onto a shortlist on the strength of that one song — the
		// hit-chasing this package exists to avoid, arriving by another route.
		return "only one loved track"
	default:
		return ""
	}
}

// Score rates one album. maxRankWeight normalises intensity across the run; a
// non-positive value disables the intensity term rather than dividing by zero.
func Score(e Evidence, maxRankWeight float64, o Options) Scored {
	s := Scored{Evidence: e, DoubleLP: e.RuntimeMin > DoubleLPMinutes}
	s.Excluded = o.exclusion(e)

	if e.TrackCount > 0 {
		s.Observed = math.Min(float64(e.lovedOnPressing())/float64(e.TrackCount), 1)
		s.Coverage = effectiveCoverage(s.Observed, e.MeanRankWeight, o.CensoringCredit)
		s.Corroboration = math.Min(float64(e.LibraryTracks)/float64(e.TrackCount), 1)
	}
	s.Persistence = persistence(len(e.Years), o)
	s.Recency = recency(e.Years, o)
	if maxRankWeight > 0 {
		s.Intensity = math.Min(e.RankWeight/maxRankWeight, 1)
	}

	w := o.Weights
	s.Score = w.Coverage*s.Coverage + w.Persistence*s.Persistence +
		w.Recency*s.Recency + w.Intensity*s.Intensity + w.Corroboration*s.Corroboration
	if e.IsCompilation {
		s.Score *= compilationPenalty
	}
	if s.Excluded != "" {
		s.Score = 0
	}
	return s
}

// DefaultRankAlpha shapes the assumed play curve. A yearly chart is not a flat
// ranking: the top track is played far more than the hundredth. Modelling that
// as a Zipf-like power law, plays(rank) ∝ rank^-alpha, 0.5 puts first place
// about ten times the hundredth — consistent with how personal listening
// actually concentrates, and far from the linear curve that would treat the
// whole list as nearly equal.
const DefaultRankAlpha = 0.5

// DefaultCensoringCredit is how much of the unheard remainder to credit; see
// effectiveCoverage. Half is deliberately conservative: the correction should
// nudge, never invent.
const DefaultCensoringCredit = 0.5

// effectiveCoverage corrects the raw share for the lists being *truncated*
// rather than complete. A yearly top-100 says nothing about rank 101, which was
// very likely played nearly as much as rank 100 — so tracks missing from the
// lists are unobserved, not unplayed, and raw coverage understates every record.
//
// The correction cannot simply assume the remainder was played, or one beloved
// track would resurrect an album nobody listened to. What licenses the
// inference is how much of the record *already* surfaced: eight tracks in the
// charts is strong evidence the other three played just below the line; one
// track is evidence of nothing but that track.
//
// So the credit scales with the *square* of the observed share. Scaling it
// linearly is not enough — a single track sitting at number one carries a mean
// rank weight high enough to out-earn a record with eight mid-chart tracks,
// which is the exact failure this correction exists to avoid. Squaring makes
// the evidence of breadth dominate the evidence of intensity. Mean rank weight
// still modulates the result, since a record whose tracks sit near the top is
// in heavier rotation than one whose tracks barely qualified, and its unheard
// remainder is correspondingly closer to the cutoff.
//
// The credit is zero at both extremes — nothing observed licenses nothing, and
// a fully observed record has no remainder — and peaks around two-thirds.
func effectiveCoverage(observed, meanRankWeight, credit float64) float64 {
	if observed <= 0 || credit <= 0 {
		return observed
	}
	inferred := (1 - observed) * observed * observed * meanRankWeight * credit
	return math.Min(observed+inferred, 1)
}

// thinlyHeard reports whether too little of a record ever surfaced for it to be
// judged as a record. The saved library is an escape hatch: the ranked lists
// truncate at 100, so a record played steadily but never obsessively barely
// appears in them, and owning most of it separately is the stronger witness.
func (e Evidence) thinlyHeard(o Options) bool {
	if o.MinLovedTracks <= 0 || e.LovedTracks >= o.MinLovedTracks {
		return false
	}
	return e.TrackCount <= 0 || float64(e.LibraryTracks)/float64(e.TrackCount) < 0.5
}

// lovedOnPressing is how many loved tracks count toward this record's coverage:
// what the pressing verifiably holds once checked, and the optimistic
// across-editions count before that.
func (e Evidence) lovedOnPressing() int {
	if e.PressingChecked {
		return e.PressingTracks
	}
	return e.LovedTracks
}

// persistence measures durability by *repeats*, not by presence: a record seen
// in a single year contains no repeat and so scores zero, however hard it was
// played that year. The square root then makes the first repeat count for more
// than the seventh, since going from one year to two is the moment a passing
// obsession becomes a relationship.
func persistence(years int, o Options) float64 {
	span := o.span()
	if span <= 1 || years <= 1 {
		return 0
	}
	return math.Sqrt(math.Min(float64(years-1)/(span-1), 1))
}

// recency decays from the album's most recent appearance, so a single year of
// obsession that is still current outranks the same obsession long abandoned.
func recency(years []int, o Options) float64 {
	if len(years) == 0 {
		return 0
	}
	last := years[0]
	for _, y := range years[1:] {
		if y > last {
			last = y
		}
	}
	age := float64(o.LastYear - last)
	if age <= 0 {
		return 1
	}
	return math.Max(0, 1-age/o.span())
}

// Rank scores every album and returns the best, honouring the per-artist cap so
// a shortlist stays a shortlist rather than one artist's discography.
func Rank(ev []Evidence, o Options) []Scored {
	maxRW := 0.0
	for _, e := range ev {
		if e.RankWeight > maxRW {
			maxRW = e.RankWeight
		}
	}
	scored := make([]Scored, 0, len(ev))
	for _, e := range ev {
		if s := Score(e, maxRW, o); s.Excluded == "" && s.Score > 0 {
			scored = append(scored, s)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Album < scored[j].Album
	})

	perArtist := map[string]int{}
	out := make([]Scored, 0, len(scored))
	for _, s := range scored {
		if o.MaxPerArtist > 0 {
			k := domain.PrimaryArtist(s.Artist)
			if perArtist[k] >= o.MaxPerArtist {
				continue
			}
			perArtist[k]++
		}
		out = append(out, s)
		if o.Size > 0 && len(out) == o.Size {
			break
		}
	}
	return out
}

// AlbumKey is a stable identity for a record across sources. It delegates to the
// domain so adapters and this service cannot disagree about what one album is.
func AlbumKey(album, artist string) string { return domain.AlbumKey(album, artist) }

// soundtrackMarkers name records made to accompany something else. They are
// excluded by default not because they are bad, but because they are usually
// shared or incidental listening — a household in a car — rather than the
// deliberate solo listening a record is bought for.
var soundtrackMarkers = []string{
	"original motion picture", "motion picture soundtrack", "original soundtrack",
	"songs from the", "original film", "music from the", "original cast",
	"(original", "original series soundtrack", "soundtrack",
}

// IsSoundtrack reports whether an album looks like a film or show soundtrack.
func IsSoundtrack(album, artist string) bool {
	l := strings.ToLower(album + " " + artist)
	for _, m := range soundtrackMarkers {
		if strings.Contains(l, m) {
			return true
		}
	}
	return false
}
