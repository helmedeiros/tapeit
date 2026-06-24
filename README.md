# tapeIt

Migrate your music library from **Spotify** to **Apple Music**.

A one-time migration CLI (Go) that reads everything you've added to your Spotify
library — owned playlists, **followed** playlists, and **Liked Songs** — matches
each track to the Apple Music catalog, and recreates the playlists in your Apple
Music library.

> Status: **working end to end** (`pull` → `match` → `push`). See
> [`docs/DESIGN.md`](docs/DESIGN.md) for architecture and
> [`docs/DECISIONS.md`](docs/DECISIONS.md) for the verified facts and trade-offs.

---

## What it does

```
spotify  ──pull──▶  local snapshot  ──match──▶  apple catalog ids  ──push──▶  apple music library
                       (sqlite)                   (isrc + fallback)
```

Four separable stages, each re-runnable and inspectable:

| Stage    | What it does                                                                 |
| -------- | ---------------------------------------------------------------------------- |
| `pull`   | OAuth into Spotify, download playlists + followed playlists + Liked Songs.   |
| `match`  | Resolve each Spotify track to an Apple Music catalog song (ISRC, then text). |
| `push`   | Create the playlists in your Apple Music library and add the matched tracks. |
| `report` | Summarize what matched, and list everything that didn't, for manual fixup.   |

`match` is the lossy step — you review the match report **before** anything is
written to Apple.

---

## ⚠️ Prerequisites & hard constraints

Read this before investing time — two of these are blockers if unmet.

### Spotify side (official, free API — but…)

- A free Spotify app registration (Client ID) at
  [developer.spotify.com](https://developer.spotify.com). Used in **development
  mode** — no quota-extension review needed for personal single-user use.
- 🔴 **The app owner (you) must have an active Spotify Premium account.** Since
  February 2026, development-mode apps do not function on a free account. You
  moved *off* Spotify — if you've dropped Premium, you must reactivate it for
  the duration of the migration.
- Development mode allows up to **5** allowlisted users and **1** Client ID —
  fine for one person.

### Apple Music side (no paid Apple Developer account — unofficial path)

The official Apple Music API requires a paid Apple Developer membership
($99/yr) to sign a developer token. We deliberately avoid that fee by using the
**Apple Music web player's own token**:

1. Log into [music.apple.com](https://music.apple.com).
2. Extract the **developer token** (a JWT bundled in the web player's JS) and
   your **`media-user-token`** cookie. `tapeIt auth apple` documents the exact
   DevTools steps.

- ⚠️ **This is an unofficial, ToS-gray-area technique.** It is confirmed to work
  in open-source projects for personal use, but it can break when Apple changes
  the web player, and both tokens expire after ~180 days (non-renewable — you
  re-extract them).
- ⚠️ The migration writes to **your own** library only. Created playlists land
  in your iCloud Music Library and sync to devices that have **Sync Library**
  enabled (verify once empirically).

### Honest alternatives

- Pay the **$99/yr** Apple Developer fee for a sanctioned, stable token.
- Use a third-party tool (Soundiiz / TuneMyMusic) — free tiers cap track/playlist
  counts. `tapeIt` exists so you own the process and have no caps.

---

## Quickstart (planned UX)

```bash
make build                                          # builds ./bin/tapeit

# 1. Spotify: authorize and pull everything (owned + followed + Liked Songs)
./bin/tapeit auth spotify --client-id <CLIENT_ID>   # browser OAuth via PKCE
./bin/tapeit pull                                   # add --owned-only to skip followed

# 2. Apple: paste the two web-player tokens (run with no args for DevTools steps)
./bin/tapeit auth apple --dev-token "<DEV_TOKEN>" --user-token "<USER_TOKEN>"

# 3. match against the Apple catalog (writes nothing to Apple yet)
./bin/tapeit match
./bin/tapeit report                                 # review matched/unmatched

# 4. recreate the playlists in Apple Music
./bin/tapeit push --dry-run                          # preview
./bin/tapeit push                                    # idempotent — safe to re-run
```

State lives under your user config dir (`tapeit/`): tokens, the library
snapshot (JSON), match results, and push progress — all outside the repo.

---

## Build & quality gates

```bash
make build      # go build ./...
make test       # go test ./... (race detector on)
make lint       # golangci-lint
make check      # fmt + vet + lint + test — must be green before every commit
```

Go 1.26+. Architecture and conventions: [`docs/DESIGN.md`](docs/DESIGN.md).

---

## License & scope

Personal-use tool for migrating **your own** accounts. Not affiliated with
Spotify or Apple. The Apple web-player token technique uses Apple's
infrastructure outside its published API terms; use it only against your own
library.
