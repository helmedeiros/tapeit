# tapeIt — Design

Hexagonal (ports & adapters) Go CLI. The domain knows nothing about Spotify,
Apple, HTTP, or SQLite; those live behind ports as adapters. This keeps the
lossy, testable core (matching, reconciliation) isolated from the brittle,
unofficial I/O at the edges.

---

## 1. Domain model

Pure value types, no I/O, no struct tags coupling them to a wire format.

```go
// A recording we want to move, as seen on the source side.
type Track struct {
    Title      string
    Artists    []string
    Album       string
    Duration   time.Duration
    ISRC       string   // primary match key; may be empty
    SourceID   string   // e.g. Spotify track URI, for traceability
}

type Playlist struct {
    Name        string
    Description string
    Kind        PlaylistKind // Owned | Followed | LikedSongs
    Tracks      []Track
}

type PlaylistKind int // Owned, Followed, LikedSongs

// Result of resolving one source Track to the target catalog.
type Match struct {
    Track       Track
    TargetID    string      // Apple catalog song id, empty if unmatched
    Confidence  Confidence  // Exact (ISRC) | High | Low | None
    Method      MatchMethod // ISRC | TextSearch | Manual
}
```

`LikedSongs` is modeled as a `Playlist` (kind `LikedSongs`) so the whole pipeline
treats it uniformly — Apple Music has no "liked songs" write API, so it becomes a
normal library playlist (default name e.g. `"Liked Songs (from Spotify)"`).

---

## 2. Ports (interfaces, owned by the domain)

```go
// Read side — the source library.
type SourceLibrary interface {
    Playlists(ctx context.Context, include IncludeSet) ([]Playlist, error)
}

// Resolve a source track to a target-catalog id.
type CatalogMatcher interface {
    Match(ctx context.Context, tracks []Track) ([]Match, error)
}

// Write side — the target library.
type TargetLibrary interface {
    CreatePlaylist(ctx context.Context, name, description string) (playlistID string, err error)
    AddTracks(ctx context.Context, playlistID string, targetIDs []string) error
    ExistingPlaylists(ctx context.Context) (map[string]string, error) // name -> id, for idempotency
}

// Durable snapshot between stages.
type SnapshotStore interface {
    SavePlaylists(ctx context.Context, pls []Playlist) error
    LoadPlaylists(ctx context.Context) ([]Playlist, error)
    SaveMatches(ctx context.Context, m []Match) error
    LoadMatches(ctx context.Context) ([]Match, error)
    MarkPushed(ctx context.Context, playlistName, targetID string) error
    Pushed(ctx context.Context) (map[string]string, error)
}
```

The **application/orchestration layer** wires these together per stage
(`pull`, `match`, `push`, `report`). It depends only on the interfaces.

---

## 3. Adapters

```
internal/
  domain/                 # types + ports above, zero dependencies
  app/                    # use cases: Pull, Match, Push, Report (orchestration)
  adapter/
    spotify/              # SourceLibrary  — HTTP + OAuth2 PKCE
    apple/                # TargetLibrary + CatalogMatcher — HTTP, amp-api
    store/                # SnapshotStore  — SQLite
  auth/
    spotify/              # PKCE loopback flow, token persistence
    apple/                # token capture + storage
cmd/tapeit/               # cobra CLI, composition root (wires adapters to app)
```

Adapters translate at the boundary: a `spotify.trackDTO` (with JSON tags) maps to
a `domain.Track`. DTOs never leak inward.

---

## 4. External API facts (verified June 2026)

These are the contracts the adapters implement. Sources in `docs/DECISIONS.md`.

### Spotify (source) — official, dev mode

- Auth: **Authorization Code + PKCE**. Loopback redirect **`http://127.0.0.1:PORT/callback`**
  — `localhost` is rejected.
- Scopes: `playlist-read-private`, `playlist-read-collaborative`, `user-library-read`.
- `GET /v1/me/playlists` — returns playlists **owned or followed** by the user
  (this is how we get followed playlists; there's no separate endpoint).
  Collaborative playlists owned by *other* users are excluded.
- `GET /v1/playlists/{id}/items` — playlist tracks. **Note the rename**: it is
  `/items`, not the old `/tracks`. Paginate (100/page).
- `GET /v1/me/tracks` — Liked Songs. Paginate (50/page).
- ISRC: `track.external_ids.isrc`, present inline on the above — **do not** use
  `GET /v1/tracks?ids=` to enrich (removed from dev mode). ISRC may be empty for
  some tracks → fallback matcher.
- Rate limits: 30s rolling window, `429` + `Retry-After`. Honor it.

### Apple Music (target + catalog) — unofficial web-player token

- Host: **`https://amp-api.music.apple.com`** (not `api.music.apple.com`).
- Headers:
  - `Authorization: Bearer <web-player-developer-token>` (all calls)
  - `Music-User-Token: <media-user-token>` (only for `/v1/me/...` library/write)
  - `Origin: https://music.apple.com` (**required** — amp-api enforces CORS)
  - `Content-Type: application/json` (writes)
- Storefront: `GET /v1/me/storefront` → `data[0].id` (e.g. `"de"`). Requires the
  user token; allow manual override in config to avoid that dependency.
- **Match by ISRC**: `GET /v1/catalog/{sf}/songs?filter[isrc]={isrc}` — dev token
  only. Accepts comma-separated ISRCs, **max 25 songs per response**. One ISRC
  can return multiple songs → disambiguate by `durationInMillis` ≈ source
  duration (±~2s), then album, then artist/title. Handle the known case where a
  returned id 404s on follow-up — fall through to the next candidate / text search.
- **Fallback search**: `GET /v1/catalog/{sf}/search?types=songs&term=...&limit=25`
  — dev token only. Results carry the same attributes (incl. `durationInMillis`,
  `isrc`) for scoring.
- **Create playlist**: `POST /v1/me/library/playlists`, expect `201`:
  ```json
  { "attributes": { "name": "…", "description": "…" },
    "relationships": { "tracks": { "data": [ { "id": "…", "type": "songs" } ] } } }
  ```
  `relationships` is optional at creation.
- **Add tracks**: `POST /v1/me/library/playlists/{id}/tracks`, expect `201`:
  ```json
  { "data": [ { "id": "…", "type": "songs" } ] }
  ```
  Chunk to ~100 ids per request (no documented write cap — defensive). `429`
  backoff.
- Both tokens expire ~180 days, non-renewable. Detect `401/403` → prompt
  re-extraction.

---

## 5. Matching strategy

Per track, in order, stop at first acceptable:

1. **ISRC exact** — if `Track.ISRC != ""`, query `filter[isrc]`. If one result →
   `Exact`. If several → pick closest duration/album → `Exact`/`High`.
2. **Text search fallback** — `term = "{title} {primary artist}"`, score candidates
   on normalized title + artist + duration. Above threshold → `High`/`Low`.
3. **No match** — record as `None`, surface in the report.

Batch ISRC lookups (≤25) to cut request count. Cache catalog responses in the
snapshot so re-running `match` is cheap.

---

## 6. Idempotency & re-runs

- `pull` overwrites the snapshot (source is the truth).
- `match` is a pure function of the snapshot + catalog; cacheable, repeatable.
- `push` checks `Pushed()` and `ExistingPlaylists()` before creating — a playlist
  already created (by name) is skipped, and partial track-adds resume from where
  they stopped. Re-running after a token expiry / crash is safe.

---

## 7. CLI surface (cobra)

```
tapeit auth spotify
tapeit auth apple
tapeit pull   [--include owned,followed,liked] [--owned-only]
tapeit match  [--threshold 0.8]
tapeit push   [--dry-run]
tapeit report [--final] [--format md|csv]
```

`cmd/tapeit` is the only place adapters are constructed and injected — the
composition root. Everything else depends on `domain` ports.

---

## 8. Testing & quality gates

- **Domain & app**: pure unit tests with fakes implementing the ports (no HTTP).
  Matching/scoring and reconciliation get table-driven tests — this is where bugs
  hide.
- **Adapters**: test against recorded HTTP fixtures (golden files); no live calls
  in CI.
- Gates (green before every commit): `gofmt`, `go vet`, `golangci-lint`,
  `go test -race ./...`.
- Small, focused, independently-revertable commits; Conventional Commits.

---

## 9. Build order (small commits / vertical slices)

1. Repo skeleton: module, `Makefile`, `golangci-lint` config, CI, empty `domain`.
2. Domain types + ports + their unit tests (fakes).
3. `store` (SQLite) adapter + `SnapshotStore` tests.
4. Spotify auth (PKCE loopback) — `tapeit auth spotify`.
5. Spotify `SourceLibrary` adapter + fixtures — `tapeit pull`.
6. Apple auth capture — `tapeit auth apple`.
7. Apple `CatalogMatcher` (ISRC then search) + fixtures — `tapeit match`.
8. `report`.
9. Apple `TargetLibrary` (create + add, idempotent) — `tapeit push` (`--dry-run` first).
10. End-to-end dry run against a real small library; verify device sync once.

Each step ends green and is usable on its own.
