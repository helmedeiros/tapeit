// Package musicapp reads play counts from the local Music app.
//
// It exists because the Apple Music API does not expose them. The service knows
// what a listener recently played and what is in heavy rotation, but not how
// many times each track has been reached for — and that count is the only
// untruncated record of what someone actually listens to.
//
// The app is scripted rather than read from its database: Music.app keeps its
// library in a private binary format that changes between releases, while the
// scripting interface is a published contract.
package musicapp

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/helmedeiros/tapeit/internal/domain"
)

// fieldSep separates fields in a scripted row. A tab is used because it cannot
// occur inside a track, artist or album name, where every printable separator
// eventually does.
const fieldSep = "\t"

// script asks for one row per played track. Tracks never played are filtered in
// the query rather than here, so a large library does not cross the boundary
// only to be discarded.
const script = `tell application "Music"
  set out to ""
  repeat with t in (tracks of library playlist 1 whose played count > 0)
    set out to out & (played count of t) & tab & (name of t) & tab & (artist of t) & tab & (album of t) & linefeed
  end repeat
  return out
end tell`

// Reader reads play counts from the local Music app.
type Reader struct {
	run func(context.Context) (string, error)
}

// NewReader builds a reader that scripts the local Music app.
func NewReader() *Reader {
	return &Reader{run: runOsascript}
}

func runOsascript(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript", "-e", script).Output()
	if err != nil {
		return "", fmt.Errorf("read play counts from Music: %w", err)
	}
	return string(out), nil
}

// PlayCounts implements domain.PlayCountPort.
func (r *Reader) PlayCounts(ctx context.Context) ([]domain.PlayedTrack, error) {
	out, err := r.run(ctx)
	if err != nil {
		return nil, err
	}
	return parse(out), nil
}

// parse reads the scripted rows, skipping any it cannot make sense of. A
// malformed row is dropped rather than failing the read: the count is evidence
// that improves a ranking, and losing one track is a far smaller harm than
// losing the whole signal because a single album title contained something odd.
func parse(out string) []domain.PlayedTrack {
	var tracks []domain.PlayedTrack
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		parts := strings.Split(line, fieldSep)
		if len(parts) != 4 {
			continue
		}
		count, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || count <= 0 {
			continue
		}
		title := strings.TrimSpace(parts[1])
		if title == "" {
			continue
		}
		tracks = append(tracks, domain.PlayedTrack{
			Title:  title,
			Artist: strings.TrimSpace(parts[2]),
			Album:  strings.TrimSpace(parts[3]),
			Count:  count,
		})
	}
	return tracks
}

var _ domain.PlayCountPort = (*Reader)(nil)
