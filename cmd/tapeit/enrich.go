package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/helmedeiros/tapeit/internal/deezer"
	"github.com/helmedeiros/tapeit/internal/matching"
)

func cmdEnrich(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	from := fs.String("from", "", "a single playlist JSON to enrich (default: every file under --dir)")
	dir := fs.String("dir", "playlists", "directory of playlist JSON files")
	if err := fs.Parse(args); err != nil {
		return err
	}

	files, err := enrichTargets(*from, *dir)
	if err != nil {
		return err
	}

	client := deezer.NewClient()
	var searched, isrcAdded, bpmAdded int
	for _, path := range files {
		doc, err := loadDoc(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		changed := false
		for i := range doc.Tracks {
			t := &doc.Tracks[i]
			if t.Features != nil && t.Features.BPM > 0 {
				continue
			}
			searched++
			gotISRC, gotBPM, err := enrichTrack(ctx, client, t)
			if err != nil {
				return err
			}
			if gotISRC {
				isrcAdded++
				changed = true
			}
			if gotBPM {
				bpmAdded++
				changed = true
			}
		}
		if changed {
			if err := writeJSON(path, doc); err != nil {
				return err
			}
		}
		fmt.Printf("• %-45s %d tracks\n", truncate(doc.Name, 45), len(doc.Tracks))
	}

	fmt.Printf("\n✓ enriched %d playlist(s): searched %d tracks, +%d bpm, +%d isrc\n",
		len(files), searched, bpmAdded, isrcAdded)
	return nil
}

// enrichTrack matches t to a Deezer track and fills in ISRC and bpm/gain in
// place, reporting which of the two it added.
func enrichTrack(ctx context.Context, client *deezer.Client, t *playlistTrack) (isrc, bpm bool, err error) {
	cands, err := client.Search(ctx, t.Title, t.Artist)
	if err != nil {
		return false, false, fmt.Errorf("deezer search %q: %w", t.Title, err)
	}
	best, ok := pickDeezer(*t, cands)
	if !ok {
		return false, false, nil
	}
	if t.ISRC == "" && best.ISRC != "" {
		t.ISRC = best.ISRC
		isrc = true
	}
	full, err := client.Track(ctx, best.ID)
	if err != nil {
		return isrc, false, fmt.Errorf("deezer track %d: %w", best.ID, err)
	}
	if full.BPM > 0 {
		t.Features = &trackFeatures{BPM: full.BPM, Gain: full.Gain, Source: "deezer"}
		bpm = true
	}
	return isrc, bpm, nil
}

func enrichTargets(from, dir string) ([]string, error) {
	if from != "" {
		return []string{from}, nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no playlist files in %s", dir)
	}
	return files, nil
}

func loadDoc(path string) (playlistDoc, error) {
	f, err := os.Open(path)
	if err != nil {
		return playlistDoc{}, err
	}
	defer func() { _ = f.Close() }()
	var doc playlistDoc
	if err := json.NewDecoder(f).Decode(&doc); err != nil {
		return playlistDoc{}, err
	}
	return doc, nil
}

// pickDeezer chooses the candidate that matches on base title and artist, then
// closest duration. When we know the track's duration, a match more than 15s
// off is rejected as a likely different recording.
func pickDeezer(t playlistTrack, cands []deezer.Track) (deezer.Track, bool) {
	wantTitle := trackKey(t.Title)
	wantArtist := matching.Normalize(t.Artist)

	var best deezer.Track
	bestDelta, found := 1<<30, false
	for _, c := range cands {
		if trackKey(c.Title) != wantTitle {
			continue
		}
		if !artistOverlap(wantArtist, matching.Normalize(c.Artist)) {
			continue
		}
		delta := 1 << 30
		if t.DurationMS > 0 {
			delta = abs(c.DurationSec*1000 - t.DurationMS)
		}
		if !found || delta < bestDelta {
			best, bestDelta, found = c, delta, true
		}
	}
	if found && t.DurationMS > 0 && bestDelta > 15000 {
		return deezer.Track{}, false
	}
	return best, found
}

func artistOverlap(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
