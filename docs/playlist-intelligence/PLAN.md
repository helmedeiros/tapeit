# Playlist Intelligence — Feature Plan

How we could build **great playlists from a user's own saved playlists** (plus
feedback signals), in collaboration with LLMs — and how we'd know they're good.

This synthesizes the [`research/`](research/) corpus and the [`lab/`](lab/)
experiments into a concrete, testable direction. It is a living document.

---

## 1. Thesis: owned playlists are necessary but not sufficient

A user's saved playlists are a rich **taste** signal — and the [lab](lab/) shows
that structure is already visible from metadata alone (dominant artists,
recurring "connector" tracks, and artist clusters like grunge vs. indie-sleaze
that co-occur across playlists). But taste is only one of four things a *great*
playlist needs:

| Need | Have it from owned playlists? | Where it comes from |
| ---- | --- | --- |
| **Taste** (who/what) | ✅ yes — co-occurrence, artists, eras | the library itself |
| **Craft** (order, flow, transitions) | ❌ no | [research/01](research/01-what-makes-a-successful-playlist.md), [02](research/02-music-transitions-and-flow.md), [03](research/03-dj-and-radio-track-selection.md) |
| **Context** (occasion, daypart, activity, intent) | ❌ partial | explicit input + LLM theme detection |
| **A definition of success** | ❌ no | [research/04](research/04-measuring-playlist-success.md) |

Two hard constraints shape everything below (both confirmed in the research):

- **Limited audio features — but partially recoverable.** Apple Music/MusicKit
  exposes only title/artist/album metadata, and Spotify restricted its
  `audio_features` endpoint for new apps in Nov 2024. However — see
  [research/06](research/06-metadata-enrichment-sources.md) — **tempo (BPM) is
  free from the Deezer API by ISRC, and musical key is free from GetSongBPM**, so
  BPM-stepping and harmonic-mixing-lite *are* feasible via an enrichment pass
  (our exports already carry ISRCs). **Energy / valence / danceability remain the
  real gap** (need the audio file or a resolved Spotify ID). Lyrics are free via
  LRCLIB. This unlocks more of the flow layer than first assumed and feeds M3.
- **No playback telemetry.** `tapeit` writes playlists; it never observes
  skips, completions, or saves. So classic engagement metrics are unavailable
  at first — success measurement must lean on offline + LLM/human evaluation.

## 2. Signals & inputs

- **Owned playlists** — co-occurrence (track↔track, artist↔artist), artist/era
  distribution, connector tracks, and per-playlist *sequence* (order carries
  intent). Primary raw material; already exported as `playlists/*.json`.
- **Liked songs / library** — a big "co-listening" set (imported as a playlist
  today); strong for candidate generation, weak for grouping (hence the
  co-occurrence size cap in `build_db.py`).
- **Explicit intent / seeds** — a prompt ("rainy Sunday indie"), seed tracks, a
  target length/energy. Cheap and high-signal; the LLM's entry point.
- **Feedback (phased in)** — thumbs, "not this one," accept/reject of suggested
  tracks. Implicit playback signals only if a future integration provides them.

## 3. Architecture direction: three layers + an LLM

Play to each tool's strength (see [notebook 03](lab/notebooks/03-flow-eval-and-llm-ideas.ipynb)):

1. **Candidate generation (data).** From seeds/owned playlists, expand via
   co-occurrence / learned embeddings (word2vec-on-playlists → Automatic
   Playlist Continuation). "Songs that go together" comes from what the user
   already grouped. — [research/05](research/05-inferring-taste-from-libraries.md)
2. **Sequencing & flow (rules).** With no audio features, use the *computable*
   craft rules: artist/album **separation**, intentional grouping vs. scatter,
   an energy/mood arc approximated from tags/era, and the DJ "journey" /
   radio "clock" ideas as soft constraints. — [research/02](research/02-music-transitions-and-flow.md), [research/03](research/03-dj-and-radio-track-selection.md)
3. **Evaluation (data + LLM).** Score candidates before shipping (see §4).

The **LLM** does what data can't: detect **theme/intent**, **name/describe** the
playlist, write a **rationale**, fill taste gaps with justified suggestions, and
act as a **judge** against a quality rubric.

This fits `tapeit`'s hexagonal shape: a new `curator` domain service with ports
(`TasteModel`, `Sequencer`, `Evaluator`, `LLM`) and adapters, consuming the
existing `playlists/` catalog and emitting a playlist that `create`/`push` can
realise on Apple Music.

## 4. How we measure success (with no telemetry)

A tiered set, honest about what each can and can't tell us
([research/04](research/04-measuring-playlist-success.md)):

- **Offline recovery (APC holdout).** Hide the last *k* tracks of a real owned
  playlist; ask the generator to complete it; score **Recall@k / R-precision**
  (the RecSys 2018 / Million Playlist Dataset protocol). Measures "does it
  reconstruct the user's own grouping." Runnable stub in [notebook 03](lab/notebooks/03-flow-eval-and-llm-ideas.ipynb).
- **Content proxies.** Coherence (intra-playlist artist/era cohesion),
  diversity, and separation quality — computable from metadata now.
- **LLM-as-judge rubric.** Score a generated playlist 1–5 on *coherence, flow,
  intent-fit, novelty* with a one-line justification each; use as an offline
  quality gate. Known judge biases → keep a human spot-check.
- **Human spot-check.** The operator listens; qualitative, low-volume, honest.
- **Online proxies (later).** If playback signals ever exist: save/add rate,
  completion, skip rate, A/B against a baseline — with Spotify's
  success/guardrail/deterioration framing.

**Cold-start plan:** launch on offline recovery + LLM-judge + human spot-check;
add engagement metrics only if/when telemetry becomes available. Never present
LLM-judge scores as ground truth.

## 5. The lab: how we experiment

[`lab/`](lab/) makes this empirical on our real data:

- `build_db.py` → `playlists.db` (84 playlists, ~8.4k unique tracks, ~210k
  co-occurrence pairs) — import, enrich, explore with SQL/pandas.
- Notebooks: **01** library EDA, **02** co-occurrence → similarity/embeddings/APC,
  **03** metadata-only sequencing + APC-holdout evaluation + the LLM-judge sketch.

The path from lab → feature: a notebook baseline that beats the naive
same-artist recall becomes the first `TasteModel` adapter; the separation
sequencer becomes the first `Sequencer`; the LLM-judge rubric becomes the
`Evaluator`.

## 6. Phased roadmap

- **M0 — Instrument (done-ish):** export owned playlists to JSON, build the DB,
  establish the offline holdout metric as a baseline.
- **M1 — MVP curator:** seed → co-occurrence candidate generation → separation
  sequencing → LLM naming/rationale → `create`. Gate on APC-holdout + LLM-judge.
- **M2 — Better taste model:** learned embeddings; PMI/graph-walk candidate gen;
  diversity/novelty controls; intent prompt.
- **M3 — Feedback loop:** accept/reject on suggestions; personal "burn"/recency;
  optional third-party audio features to unlock true flow scoring.

## 7. ADRs to write

1. **Curator architecture** — ports/adapters for taste, sequencing, evaluation, LLM.
2. **No-audio-features stance** — commit to metadata + co-occurrence; when (if) to
   add a third-party feature source.
3. **Success metric of record** — APC-holdout + LLM-judge rubric as the gate;
   what would justify adding telemetry.
4. **LLM boundaries** — where the LLM decides vs. where deterministic data rules
   decide; reproducibility/temperature; cost.
5. **Feedback data model** — what signals we store and how they feed the loop.

## 8. Open questions / risks

- Can co-occurrence alone generate *fresh* (non-obvious) yet coherent picks, or
  do we need embeddings/external catalog signals for discovery?
- Is APC-holdout a good proxy for *subjective* quality, or does it just reward
  reconstructing the past (anti-novelty)? Pair it with novelty metrics.
- LLM-judge bias and cost; how much human-in-the-loop is sustainable.
- Ethical/UX: avoiding filter-bubble reinforcement (research/05) — bake a
  novelty/serendipity dial into the objective, not as an afterthought.
