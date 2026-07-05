package main

import (
	"flag"
	"fmt"

	"github.com/helmedeiros/tapeit/internal/curator"
)

func cmdSequence(args []string) error {
	fs := flag.NewFlagSet("sequence", flag.ContinueOnError)
	from := fs.String("from", "", "playlist JSON to reorder (required)")
	flow := fs.String("flow", "smooth", "tempo shape: smooth|arc")
	out := fs.String("out", "", "write reordered playlist here (default: overwrite --from)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *from == "" {
		return fmt.Errorf("give --from FILE (a playlist JSON, ideally enriched with bpm)")
	}
	mode, ok := curator.ParseFlow(*flow)
	if !ok || mode == curator.FlowNone {
		return fmt.Errorf("unknown --flow %q (use smooth|arc)", *flow)
	}

	doc, err := loadDoc(*from)
	if err != nil {
		return err
	}
	tracks := toCuratorTracks(doc.Tracks)
	withBPM := 0
	for _, t := range tracks {
		if t.BPM > 0 {
			withBPM++
		}
	}
	doc.Tracks = fromCuratorTracks(curator.Sequence(tracks, mode))

	path := *out
	if path == "" {
		path = *from
	}
	if err := writeJSON(path, doc); err != nil {
		return err
	}

	fmt.Printf("✓ sequenced %q (%s) — %d/%d tracks have bpm → %s\n", doc.Name, *flow, withBPM, len(tracks), path)
	if withBPM < len(tracks) {
		fmt.Println("  tracks without bpm are appended after the tempo run — run `tapeit enrich` first for a full curve")
	}
	return nil
}
