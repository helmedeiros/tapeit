---
name: create-apple-playlist
description: Use this skill to create an Apple Music playlist from a hand-supplied song list — typically transcribed from Spotify screenshots, a "This Is <Artist>" playlist, a screenshot/photo of a tracklist, or any list of "Title - Artist" songs the user pastes. Trigger on phrases like "create this playlist on apple music", "recreate this on apple", "make an apple music playlist from these songs", or when the user drops playlist screenshots and asks to build it. Builds a JSON track list and runs `tapeit create` to match and push the playlist into the user's Apple Music library.
version: 1.0.0
---

# Create an Apple Music playlist from a song list

Recreate a playlist in the user's Apple Music library from a hand-supplied list of
songs (often transcribed from Spotify screenshots). This is a workflow over the
`tapeit create` command in this repo — it does **not** need the full Spotify
`pull`/`match`/`push` pipeline, only the standalone iTunes-Search-based `create`
path.

## When this skill applies
- The user shares one or more screenshots/photos of a playlist (Spotify, Apple, a
  printed tracklist) and asks to build it on Apple Music.
- The user pastes a list of songs and wants them turned into an Apple Music playlist.
- The user references an existing `this-is-*.json` style export to (re)create.

## Prerequisites
- The `tapeit` binary built at `./bin/tapeit` (run `make build` if missing).
- Apple Music credentials already saved (`apple_creds.json` under the tapeit config
  dir — `~/Library/Application Support/tapeit/` on macOS). If `tapeit create`
  reports missing/invalid creds, tell the user to run `tapeit auth apple` with the
  two web-player tokens (see the repo README); do not try to fabricate them.

## Workflow

### Step 1 — Transcribe the song list
From the screenshots or pasted text, build the full ordered list of tracks. For
each track capture **title** and **artist**. Notes:
- Strip remaster/version suffixes that hurt search matching, e.g.
  `Lonely Boy - 2021 Remaster` → `Lonely Boy`. The matcher is search-only (no ISRC),
  so a clean canonical title matches best.
- Drop `(feat. X)` from the title when it risks a miss; the base title usually
  resolves fine.
- Keep apostrophes, slashes, and parentheses that are part of the real title
  (`Lo/Hi`, `Who's Been Foolin' You`, `Beautiful People (Stay High)`).
- For a "This Is <Artist>" playlist every artist is the same; still set `artist`
  on each track (improves match precision), including any guest-billed rows.

### Step 2 — Write the JSON track list
Write a file under `playlists/` named `<slug>.json` (the service-agnostic
location `tapeit export` also uses) in this shape — the `name` field becomes the
Apple Music playlist name:

```json
{
  "name": "This Is The Black Keys",
  "tracks": [
    {"title": "Lonely Boy", "artist": "The Black Keys"},
    {"title": "Howlin' for You", "artist": "The Black Keys"}
  ]
}
```

### Step 3 — Dry-run to review matching
```bash
./bin/tapeit create --from playlists/<slug>.json --dry-run
```
Read the match summary. Aim for ~100% matched. If any tracks are `unmatched` or the
`low` count is high, fix those titles in the JSON (re-check spelling, drop suffixes,
try the album single name) and re-run the dry-run. The command can take ~20-40s for
50 tracks — run it in the background and wait for completion rather than polling.

### Step 4 — Create the playlist
Once the dry-run looks good:
```bash
./bin/tapeit create --from playlists/<slug>.json
```
This is idempotent — re-running resumes and won't duplicate tracks. If a playlist of
the same name already exists but tapeit didn't create it, add `--adopt` to fill it
(warn the user this can mix with manually-added tracks). On success it prints
`✓ Created.`

### Step 5 — Report back
Tell the user the playlist name, how many of N tracks matched, and flag any track
that didn't match or matched at low confidence so they can verify it in Apple Music.

## Notes
- `--name` overrides the JSON's `name`; for a plain text list (`Title - Artist` per
  line) `--name` is required since text carries no name.
- The created playlist lands in the user's iCloud Music Library and syncs to devices
  with Sync Library enabled.
