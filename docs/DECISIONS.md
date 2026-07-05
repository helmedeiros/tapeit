# Decisions & verified facts

## Settled (this planning session)

| Decision            | Choice                                                            |
| ------------------- | ---------------------------------------------------------------- |
| Direction           | Spotify → Apple Music, one-time migration                        |
| Scope               | Owned playlists **+ followed playlists + Liked Songs**           |
| Interface           | CLI                                                              |
| Language            | Go (1.26+)                                                       |
| Apple access        | **No** paid Developer account — web-player token (unofficial)    |
| Repository          | Private, separate from dotfiles                                  |
| Architecture        | Hexagonal, SOLID, clean code, high quality gates, small commits  |

## Open questions

- **Spotify Premium**: dev-mode apps require the owner to have active Premium
  (Feb 2026). Confirm you have/will reactivate it — otherwise the whole source
  side is blocked.
- **Liked Songs target name / split**: single playlist `"Liked Songs (from
  Spotify)"`, or leave configurable? (Default: single playlist.)
- **Followed playlists**: recreate as your own library copies (only option via
  the API). Confirm that's the desired behavior vs. skipping them.
- **Device-sync verification**: do an early empirical check that a
  web-token-created playlist appears on an iPhone with Sync Library on.

## Verified facts (June 2026) — sources

### Apple write via web-player token — CONFIRMED viable
- Working OSS reference: <https://github.com/Myp3a/apple-music-api> (creates
  library playlists + adds tracks with Bearer dev token + `media-user-token`).
- Constraints + cookie name + write rules: <https://www.music-assistant.io/music-providers/apple-music/>
- Token extraction methods: <https://github.com/OrfiDev/orpheusdl-applemusic-basic/blob/main/applemusic_api.py>
- Host `amp-api.music.apple.com`, requires `Origin: https://music.apple.com`.
- Tokens expire ~180 days, non-renewable; technique is brittle / ToS gray area.
- Request shapes: Apple docs `libraryplaylistcreationrequest`,
  `add-tracks-to-a-library-playlist`.

### Spotify reads — CONFIRMED available in dev mode
- 🔴 App owner must have **Premium** (Feb 2026):
  <https://developer.spotify.com/documentation/web-api/concepts/quota-modes>
- Feb 2026 changes (5-user allowlist, `/playlists/{id}/items` rename,
  `/users/{id}/playlists` removed, `/tracks?ids=` removed from dev mode):
  <https://developer.spotify.com/documentation/web-api/references/changes/february-2026>
- `/me/playlists` returns owned **and** followed:
  <https://developer.spotify.com/documentation/web-api/reference/get-a-list-of-current-users-playlists>
- Nov 2024 deprecations don't touch personal-library reads:
  <https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api>
- PKCE + loopback, `127.0.0.1` only (no `localhost`):
  <https://developer.spotify.com/documentation/web-api/concepts/redirect_uri>
- `external_ids.isrc` intact (removal proposed Feb 2026, **reverted** Mar 2026 —
  keep monitoring + fallback matcher):
  <https://developer.spotify.com/documentation/web-api/references/changes/march-2026>

### Apple ISRC matching — CONFIRMED
- `filter[isrc]`, comma-separated, max 25 songs/response, one ISRC → many songs:
  <https://developer.apple.com/documentation/applemusicapi/get-multiple-catalog-songs-by-isrc>
- Catalog reads need dev token only (no user token).
- `/v1/me/storefront` for storefront id (needs user token; allow manual override).
- Text-search fallback carries `durationInMillis` + `isrc` for scoring.
- Expect a non-trivial unmatched rate (regional/catalog gaps, ISRC variance,
  occasional 404 on a returned id) — surface for manual review.

### Metadata enrichment (bpm/isrc) — CONFIRMED via Deezer
- `tapeit enrich` fills `features.bpm`/`gain` and backfills `isrc` on the
  playlist JSON, matching by title/artist search. Public Deezer API, no auth.
- Search: `GET /search/track?q=artist:"A" track:"T"` returns id + isrc +
  duration (no bpm). Full track: `GET /track/{id}` returns `bpm`, `gain`, `isrc`.
  <https://developers.deezer.com/api/track>
- Rationale: our Apple-library reads carry no ISRC, and Apple/Spotify expose no
  usable audio features (Spotify `audio_features` restricted for new apps Nov
  2024). Deezer is the free path to bpm + isrc. See
  `docs/playlist-intelligence/research/06-metadata-enrichment-sources.md`.
- Known gap: Deezer reports `bpm: 0` for some tracks (older/live); energy /
  valence / key remain unavailable without audio or a resolved Spotify id.
- Self-throttled ~4 req/s; two calls per track (search + track), so a full
  library pass is minutes — run per-file or as a background sweep.

### Curator (`tapeit curate`) — co-occurrence, not PMI
- Builds a playlist from the user's own library by expanding from a seed artist
  along artist co-occurrence (which artists they group together across
  playlists), then separating so no two adjacent tracks share an artist.
- Neighbours ranked by **focus-weighted affinity**, capped to the top
  `--breadth` (default 12). Each playlist distributes one unit of affinity, so a
  pair in a k-artist playlist scores 1/(k-1) — co-occurrence in a tight 12-artist
  set counts far more than in a 150-artist grab-bag. A raw shared-playlist count
  still gates `--min-affinity`. This tightened Arctic Monkeys (added Wolf Alice /
  Belle & Sebastian, dropped John Mayer) and Nirvana (Smashing Pumpkins / Incubus
  over 3 Doors Down) while leaving the already-good jazz cluster unchanged.
- Tried **PMI** first — it backfired: artists in a single playlist (freq 1) get
  inflated PMI and tie, so one-off co-occurrences flooded the top alphabetically
  (Arctic → "3 Doors Down, ABBA"). Focus-weighting is the robust alternative on
  this sparse, "This Is <Artist>"-heavy library.
- **Empirically validated** (leave-one-out APC eval, `lab/experiments/`): raw &
  focus recover 2–3× more held-out artists than a popularity baseline; unsmoothed
  PMI ≈ popularity and even count-gated PMI loses to raw/focus (avoiding PMI is
  vindicated, not a tuning accident); focus beats raw ~30% in the **single-seed**
  case `curate` actually uses. But single-seed co-occurrence is a *weak* regime
  (a lone seed under-recovers vs popularity) → motivates **multi-artist seeding**
  and keeps `--discover` well-justified. See `lab/experiments/RESULTS.md`.
- Known limit: sparse seeds only present in one dedicated playlist + generic
  hits mixes (e.g. Daft Punk) expand into whatever co-occurs in those mixes.
  Lower `--breadth` to stay tighter (more seed tracks, fewer neighbours).
- Better candidate generation (embeddings, playlist-size-weighted affinity) is
  future work — see docs/playlist-intelligence/PLAN.md and lab notebook 02.

### Structural cohesion — tried and rejected (like PMI)
- Hypothesis: co-occurrence can drift into an unrelated cluster via a "bridge"
  artist, so re-weight neighbours by how embedded each is in the seed's
  neighbourhood (a true cluster-mate co-occurs with the seed's *other*
  neighbours; a bridge doesn't). Two formulations tried: raw weighted
  common-neighbour mass, and the neighbourhood-internal *fraction* of a
  candidate's connections (normalized, to avoid favoring hubs).
- **Both hurt.** Swept the blend weight against `--evaluate`: artist recall was
  flat-to-worse and fell monotonically as the weight rose (raw: 0.143 → 0.113;
  normalized: 0.143 → 0.083), track recall never improved. Rejected — not
  shipped, mirroring the PMI decision.
- Why it fails *here*: the library is sparse and star-shaped (dedicated "This Is
  <Artist>" playlists), so genuinely related artists frequently connect to a seed
  through a single playlist and have few common neighbours. Cohesion assumes a
  dense community graph this data doesn't have, and demotes exactly the
  sparse-but-real related artists. A content signal (genre/tags) would attack
  bridging without this assumption — that's the remaining candidate, needing an
  external source, so it stays future work.

### Curate seeding — multi-artist and whole-playlist (APC)
- `--seed` takes a comma-separated artist list; affinity is summed across all
  seeds. Empirically single-seed co-occurrence under-recovers vs popularity, but
  seeding from several artists is materially stronger (see the APC eval above), so
  multi-seed is the intended mode.
- `--seed-playlist <slug|path>` is the strongest case: seed from **all** of an
  existing playlist's artists and **exclude its own tracks**, so the result is
  genuine Automatic Playlist Continuation — "more like this, but new". A bare slug
  resolves under `--dir`; a value ending in `.json` or containing a path separator
  is used as a path. Names the output `"More Like <source>"`.
  Verified: `--seed-playlist indie-rock-club` → 18 tracks, 0 overlapping the
  source, expanding into Editors / Foals / The Wombats / Metric.
- `--discover` fans out at most `maxDiscoverySeeds` (5) seeds online — a whole
  seed-playlist can carry hundreds of artists, one Deezer lookup each.

### Track selection — favorite-ranked, not alphabetical (major quality fix)
- Curate used to pick each artist's tracks **alphabetically by title**, which
  surfaced deep cuts over signatures (Black Keys → "10 Lovers" not "Lonely Boy";
  Strokes → "At The Door" not "Last Nite"; Interpol → "Anywhere" not "Evil").
  Right artists, wrong songs — the most *visible* failure, and one `--evaluate`
  can't see because it scores artists, not tracks.
- Fixed by ranking each artist's pool by the user's revealed preference:
  **(1) playlist frequency** (a song saved across more of your playlists is one
  you love), **(2) earliest source position** (streaming "This Is" lists are
  hit-ordered — Lonely Boy is track 1, and we were throwing that order away),
  then **(3) title**. Zero new deps. Every Arctic-Monkeys-seed pick flipped from
  a deep cut to a signature (Do I Wanna Know?, Seven Nation Army, Last Nite,
  Lonely Boy, Evil, Maps).
- Also collapses near-duplicate releases (`baseTitle` strips "- Remastered",
  "(Live)", "(feat. …)", "- Radio Edit", etc.) so a generated playlist never
  places two versions of the same song, preferring the clean studio title as the
  representative. Conservative marker list + token-prefix matching so "olive"
  isn't mistaken for "live" and "Beautiful People (Stay High)" survives.

### BPM-aware sequencing (`tapeit sequence`, `curate --flow`)
- Orders a playlist by tempo while keeping the no-adjacent-same-artist invariant.
  `smooth` ramps BPM from slowest to fastest; `arc` climbs to a peak in the middle
  then eases down (bitonic). Tracks with no BPM can't sit on the curve, so they're
  artist-separated among themselves and appended after the tempo run.
- Realistic pipeline is **curate → enrich → sequence**: our Apple/library reads
  carry no BPM, so tempo data only exists after `tapeit enrich` (Deezer). Hence a
  standalone `tapeit sequence --from FILE --flow smooth|arc` (the usual path, run
  post-enrich) *and* a `curate --flow` flag (useful once library tracks are
  enriched). With too little BPM signal (<2 tracks) it falls back to plain artist
  separation, so it's always safe to pass.
- Artist de-clumping wins ties over strict tempo monotonicity: `repairAdjacent`
  swaps a same-artist neighbour for the nearest later different-artist track,
  which introduces small local BPM inversions by design (verified on the dinner-
  party set: a clean 101→172 ascent with a handful of ±2 BPM repairs). Same-artist
  tracks usually share tempo, so the disturbance is minor.

### Track-level evaluation + CI gate
- `--evaluate` now measures **track** recall too, not just artists: hide a
  playlist's later *tracks*, run curate's actual pipeline from the earlier ones,
  and count how many held-out songs it recovers vs a "just add popular songs"
  baseline. This is the metric that sees the favorite-ranking fix — on the real
  library curate scores **8.48× the baseline on tracks** (0.084 vs 0.010), even
  higher than its 2.47× on artists.
- Wired as a **CI gate**: `go run ./cmd/tapeit curate --evaluate` exits non-zero
  if focus recall ever stops beating popularity (artist *or* track level), so a
  future change that quietly degrades candidate quality fails the build. Added to
  `.github/workflows/ci.yml` after build.
- Both levels reuse one leave-one-out model rebuild per test playlist; track eval
  runs the real `Curate` so the number reflects shipped behaviour, not a proxy.

### Curate self-evaluation (`--evaluate`)
- `tapeit curate --evaluate` runs the leave-one-out APC test in the binary (no
  Python, no playlist written): for each library playlist with ≥8 distinct
  artists, hide 40% of its later artists, rebuild the model from every *other*
  playlist, seed from the earlier artists, and measure Recall@20 / R-precision of
  the focus-weighted ranking against a popularity baseline.
- This is the honest confidence signal for curate *on your specific library* —
  the same protocol as `lab/experiments/affinity_eval.py`, now shippable. On the
  current library it reports focus 0.143 vs popularity 0.058 = **2.47× lift**,
  matching the offline experiment (2–3×).
- Implementation does true LOO by rebuilding the model without the test playlist
  (library is small, so O(P) rebuilds is fine and obviously correct — no
  drop-adjustment arithmetic). `Model.freq` (playlists per artist) added to back
  the popularity baseline.

### Curate overwrite guard + online discovery
- `curate` writes a *fresh* doc (unlike `import`/`enrich`, which read-merge), so
  it now refuses to overwrite an existing output file unless `--force` — a
  `--name` collision or a re-run no longer silently clobbers a real playlist.
- `--discover N` augments the library-only result with up to N tracks by similar
  artists the user does NOT already own, so curation can learn new artists.
  Similarity + top tracks come from the public Deezer API (`/search/artist` →
  `/artist/{id}/related`, `/artist/{id}/top`; no auth). Daft Punk → Cassius,
  Étienne de Crécy, Yuksek, Mr. Oizo, Alan Braxe — the electronic peers
  co-occurrence couldn't surface from this library.
- Results are cached in a local artist index (`internal/artistindex`, stored at
  config `artist_index.json`) so it's fetched once and reused offline. Already-
  owned similar artists are skipped (only genuinely new artists are added).

### LLM naming stays at the harness level — tapeit remains zero-dependency
- `tapeit` has no third-party Go dependencies, and that's a kept property (see
  go.mod, README). Rather than add the Anthropic SDK (a dependency tree) or a
  raw-HTTP client just for playlist naming, the LLM stays **out of the binary**.
- `curate` names deterministically ("Around <seed>"); an LLM (the operator's
  assistant) supplies evocative names/themes/rationale at the harness level and
  passes them via `--name`. This matches the Playlist Intelligence plan's split:
  data engine in the tool, LLM as collaborator around it.
- Revisit an in-tool `--describe` (raw-HTTP, keep zero-dep) only if naming needs
  to run unattended.
