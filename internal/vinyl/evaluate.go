package vinyl

import (
	"fmt"
	"math"
	"sort"
)

// Appearance is one track's presence in one year's ranked list. It is the raw
// observation the whole model is built from.
type Appearance struct {
	Year     int
	Rank     int // 1-based position in that year's list
	Size     int // length of that year's list, so rank weight is comparable
	Track    string
	AlbumKey string // stable identity for the album (normalised name+artist)
	Album    string // display name
	Artist   string
	// CatalogID identifies the recording, when known. It is what makes the album
	// resolvable exactly rather than by matching its name.
	CatalogID string
}

// Play is one track's play count on the service the listener actually uses now.
//
// It is a different kind of witness from a ranked list. A yearly chart holds
// only its top hundred, so a record played end to end still surfaces one or two
// tracks; the play history is not truncated and names every track that gets
// played. It covers a much shorter span, though, so it says a great deal about
// breadth and nothing at all about durability.
type Play struct {
	Track    string
	AlbumKey string
	Album    string
	Artist   string
	Count    int
}

// AlbumMeta is what the catalog knows about an album, independent of listening.
type AlbumMeta struct {
	// Name and Artist as the catalog states them. They are kept rather than
	// derived from the key: a normalised key cannot be read back into a name, so
	// an entry holding only a key can be neither displayed, re-indexed when the
	// key rule improves, nor matched to another service.
	Name          string
	Artist        string
	TrackCount    int
	RuntimeMin    int
	IsCompilation bool
	IsSoundtrack  bool
	CatalogID     string
	// UPC is the record's barcode — the identifier services agree on, and so the
	// one that makes these facts portable between them.
	UPC string
	// Genres are the catalog's own account of what a record is, kept so a cached
	// album is judged by the same evidence as a freshly resolved one.
	Genres []string
}

// RankWeight converts a position in a ranked list to an assumed share of plays,
// modelled as a Zipf-like power law: plays(rank) proportional to rank^-alpha,
// normalised so first place is 1.
//
// A ranked year is emphatically not flat — the top track is played many times
// what the hundredth is — so a linear curve understates the difference. The
// guard against this simply rediscovering "buy the album with your #1 track" is
// not a flattened curve but the low weight on intensity and the high weight on
// coverage and persistence.
func RankWeight(rank, size int, alpha float64) float64 {
	if rank < 1 {
		rank = 1
	}
	if size <= 1 || alpha <= 0 {
		return 1
	}
	return math.Pow(float64(rank), -alpha)
}

// Aggregate folds raw appearances into per-album evidence. libraryDepth counts
// distinct tracks per album key in the listener's separately saved library.
// albumEvidence accumulates one record's evidence while it is being gathered.
type albumEvidence struct {
	ev     Evidence
	tracks map[string]struct{} // charted
	played map[string]struct{} // played, once admitted
	years  map[int]struct{}
	ranked int // how many chart appearances, for the mean rank weight
}

// evidenceSet gathers records as each source names them.
type evidenceSet map[string]*albumEvidence

func (s evidenceSet) get(key, album, artist string) *albumEvidence {
	e, ok := s[key]
	if !ok {
		e = &albumEvidence{
			ev:     Evidence{Album: album, Artist: artist},
			tracks: map[string]struct{}{}, played: map[string]struct{}{}, years: map[int]struct{}{},
		}
		s[key] = e
	}
	return e
}

// addAppearances folds in what the ranked lists say.
func (s evidenceSet) addAppearances(apps []Appearance, o Options) {
	for _, a := range apps {
		if a.AlbumKey == "" {
			continue
		}
		e := s.get(a.AlbumKey, a.Album, a.Artist)
		e.tracks[a.Track] = struct{}{}
		e.years[a.Year] = struct{}{}
		e.ev.RankWeight += RankWeight(a.Rank, a.Size, o.RankAlpha)
		e.ranked++
	}
}

// addPlays folds in what the play history says, keeping it only for records the
// listener came back to.
func (s evidenceSet) addPlays(plays []Play, o Options) {
	replayed := map[string]int{}
	for _, p := range plays {
		if p.AlbumKey == "" || p.Count <= 0 {
			continue
		}
		e := s.get(p.AlbumKey, p.Album, p.Artist)
		e.played[p.Track] = struct{}{}
		e.ev.Plays += p.Count
		if p.Count > 1 {
			replayed[p.AlbumKey]++
		}
	}
	for key, e := range s {
		e.ev.ReturnedTo, e.played = admitPlays(e.played, replayed[key], o)
	}
}

// finish resolves one record's gathered evidence into the value the model scores.
func (e *albumEvidence) finish(key string, meta map[string]AlbumMeta, libraryDepth map[string]int) Evidence {
	e.ev.PlayedTracks = len(e.played)
	loved := make(map[string]struct{}, len(e.tracks)+len(e.played))
	for tr := range e.tracks {
		loved[tr] = struct{}{}
	}
	for tr := range e.played {
		loved[tr] = struct{}{}
	}
	e.ev.LovedTracks = len(loved)
	for tr := range loved {
		e.ev.LovedTitles = append(e.ev.LovedTitles, tr)
	}
	sort.Strings(e.ev.LovedTitles)
	if e.ranked > 0 {
		e.ev.MeanRankWeight = e.ev.RankWeight / float64(e.ranked)
	}
	for y := range e.years {
		e.ev.Years = append(e.ev.Years, y)
	}
	sort.Ints(e.ev.Years)
	e.ev.LibraryTracks = libraryDepth[key]
	if m, ok := meta[key]; ok {
		e.ev.TrackCount, e.ev.RuntimeMin = m.TrackCount, m.RuntimeMin
		e.ev.IsCompilation, e.ev.IsSoundtrack = m.IsCompilation, m.IsSoundtrack
		e.ev.CatalogID = m.CatalogID
	}
	return e.ev
}

// Aggregate folds every source of evidence into one record per album: what the
// ranked lists charted, what the play history says was actually reached for,
// and how much of it the listener separately saved.
func Aggregate(apps []Appearance, plays []Play, meta map[string]AlbumMeta,
	libraryDepth map[string]int, o Options) []Evidence {
	set := evidenceSet{}
	set.addAppearances(apps, o)
	set.addPlays(plays, o)

	out := make([]Evidence, 0, len(set))
	for key, e := range set {
		out = append(out, e.finish(key, meta, libraryDepth))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Album < out[j].Album })
	return out
}

// admitPlays decides whether a record's play history counts as evidence of
// devotion, and discards it when it does not.
//
// A record played end to end once leaves every one of its tracks at a single
// play. Counted by distinct tracks that is indistinguishable from devotion —
// every track present, perfect coverage — which is how records a listener
// merely auditioned reached a shortlist meant for records they live with.
// Coming back to it is the difference, and the counts record it.
func admitPlays(played map[string]struct{}, replayed int, o Options) (bool, map[string]struct{}) {
	returned := replayed >= o.MinReplayedTracks
	if len(played) > 0 && !returned {
		return false, map[string]struct{}{}
	}
	return returned, played
}

// YearResult is one held-out year's outcome.
type YearResult struct {
	Year          string
	ModelRecall   float64 // share of that year's tracks on albums the model picked
	BaseRecall    float64 // same, for a naive "albums of your top tracks" shortlist
	ModelHits     int
	BaseHits      int
	HeldOutTracks int
}

// Report is the outcome of leave-one-year-out evaluation.
type Report struct {
	Years []YearResult
	Lift  float64 // model recall / baseline recall, pooled over all years
}

func (r Report) String() string {
	s := "leave-one-year-out (share of the held-out year's tracks on shortlisted albums)\n"
	for _, y := range r.Years {
		s += fmt.Sprintf("  %s  model %5.1f%%  baseline %5.1f%%  (%d vs %d of %d tracks)\n",
			y.Year, y.ModelRecall*100, y.BaseRecall*100, y.ModelHits, y.BaseHits, y.HeldOutTracks)
	}
	if math.IsInf(r.Lift, 1) {
		s += "  → baseline recovered nothing the model did\n"
	} else {
		s += fmt.Sprintf("  → %.2fx the baseline\n", r.Lift)
	}
	return s
}

// Evaluate hides one year at a time, ranks albums on the remaining years, and
// measures how much of the hidden year those albums account for — against a
// baseline that simply takes the albums of the highest-ranked tracks.
//
// It answers the only question that matters about the weights: does modelling
// coverage and persistence beat just following the hits? A lift at or below 1
// means the extra machinery earns nothing and the weights are wrong.
func Evaluate(apps []Appearance, meta map[string]AlbumMeta, libraryDepth map[string]int, o Options) Report {
	years := map[int]struct{}{}
	for _, a := range apps {
		years[a.Year] = struct{}{}
	}
	ordered := make([]int, 0, len(years))
	for y := range years {
		ordered = append(ordered, y)
	}
	sort.Ints(ordered)

	var rep Report
	var mHits, bHits, total int
	for _, held := range ordered {
		var train, test []Appearance
		for _, a := range apps {
			if a.Year == held {
				test = append(test, a)
			} else {
				train = append(train, a)
			}
		}
		if len(test) == 0 || len(train) == 0 {
			continue
		}

		picked := map[string]struct{}{}
		for _, s := range Rank(Aggregate(train, nil, meta, libraryDepth, o), o) {
			picked[albumKeyOf(train, s.Album, s.Artist)] = struct{}{}
		}
		base := baselineAlbums(train, meta, o)

		var m, b int
		for _, a := range test {
			if _, ok := picked[a.AlbumKey]; ok {
				m++
			}
			if _, ok := base[a.AlbumKey]; ok {
				b++
			}
		}
		rep.Years = append(rep.Years, YearResult{
			Year: fmt.Sprint(held), ModelHits: m, BaseHits: b, HeldOutTracks: len(test),
			ModelRecall: float64(m) / float64(len(test)), BaseRecall: float64(b) / float64(len(test)),
		})
		mHits, bHits, total = mHits+m, bHits+b, total+len(test)
	}
	switch {
	case bHits > 0:
		rep.Lift = float64(mHits) / float64(bHits)
	case mHits > 0:
		rep.Lift = math.Inf(1) // baseline found nothing the model did find
	default:
		rep.Lift = 0
	}
	_ = total
	return rep
}

// baselineAlbums is the naive shortlist: the albums holding the highest-ranked
// individual tracks, subject to the same practical filters so the comparison is
// about ranking logic rather than about excluding singles.
func baselineAlbums(train []Appearance, meta map[string]AlbumMeta, o Options) map[string]struct{} {
	best := map[string]int{}
	for _, a := range train {
		if r, ok := best[a.AlbumKey]; !ok || a.Rank < r {
			best[a.AlbumKey] = a.Rank
		}
	}
	type kv struct {
		key  string
		rank int
	}
	all := make([]kv, 0, len(best))
	for k, r := range best {
		m := meta[k]
		if m.TrackCount > 0 && m.TrackCount < o.MinTracks {
			continue
		}
		if m.IsSoundtrack && !o.IncludeSoundtracks {
			continue
		}
		all = append(all, kv{k, r})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].rank != all[j].rank {
			return all[i].rank < all[j].rank
		}
		return all[i].key < all[j].key
	})
	out := map[string]struct{}{}
	for i, e := range all {
		if o.Size > 0 && i >= o.Size {
			break
		}
		out[e.key] = struct{}{}
	}
	return out
}

// albumKeyOf recovers an album's key from its display fields.
func albumKeyOf(apps []Appearance, album, artist string) string {
	for _, a := range apps {
		if a.Album == album && a.Artist == artist {
			return a.AlbumKey
		}
	}
	return album + "|" + artist
}
