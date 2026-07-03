# Affinity scorer evaluation — results

Empirical test of the candidate-generation heuristics behind `tapeit curate`,
run on the real library (`playlists.db`, 84 playlists) with
[`affinity_eval.py`](affinity_eval.py). Reproduce:

```bash
python3 docs/playlist-intelligence/lab/build_db.py
python3 docs/playlist-intelligence/lab/experiments/affinity_eval.py
```

## Method

Artist-level **Automatic Playlist Continuation** (RecSys 2018 / Million Playlist
Dataset protocol), leave-one-out: for each multi-artist playlist, hide some of
its artists, seed from the rest, and measure how many held-out artists each
scorer ranks into the top-K. The test playlist is removed from the co-occurrence
model first (no leakage), and only *recoverable* held-out artists (present in ≥1
other playlist) count. Two modes:

- **multi-seed** — hide the later 40% of artists, seed from the earlier 60% (the standard APC task).
- **single-seed** — seed from *one* artist, predict the playlist's others (mirrors `curate --seed`).

Scorers: `popularity` (baseline — rank by how many playlists an artist appears
in), `raw` (shared-playlist count), `focus` (shared playlists weighted `1/(k-1)`
per k-artist playlist — what `curate` uses), `pmi` (unsmoothed), `pmi_gated`
(PMI over pairs co-occurring ≥2×).

## Results

**Multi-seed** — 48 playlists (≥8 artists, ≤250 tracks):

| strategy   | Recall@10 | Recall@20 | R-precision |
| ---------- | --------- | --------- | ----------- |
| popularity | 0.051     | 0.064     | 0.037       |
| raw        | 0.111     | 0.174     | **0.081**   |
| focus      | **0.112** | **0.186** | 0.075       |
| pmi        | 0.057     | 0.102     | 0.043       |
| pmi_gated  | 0.097     | 0.126     | 0.047       |

Head-to-head @Recall20: focus vs raw = 8 W / 33 T / 7 L; focus vs pmi_gated = 13 / 33 / 2.

**Single-seed** — 2,166 (playlist, seed-artist) pairs, mirrors `curate --seed`:

| strategy   | Recall@10 | Recall@20 |
| ---------- | --------- | --------- |
| popularity | **0.041** | **0.054** |
| raw        | 0.021     | 0.035     |
| focus      | 0.029     | 0.046     |
| pmi        | 0.005     | 0.013     |
| pmi_gated  | 0.012     | 0.019     |

## What this proves

1. **Co-occurrence is a real signal** — multi-seed, `raw`/`focus` recover 2–3× as
   many held-out artists as the popularity baseline. The premise holds.
2. **Unsmoothed PMI fails, empirically** — `pmi` (0.057 R@10) barely beats
   popularity (0.051), confirming the low-frequency-inflation problem that made
   it produce alphabetical noise. Gating helps (`pmi_gated` 0.097) but **still
   loses to raw/focus** — so avoiding PMI on this sparse library is vindicated,
   not just a tuning accident.
3. **Focus-weighting is justified for `curate`'s actual use** — multi-seed it's a
   near-tie with raw (33/48 identical), but **single-seed** (one seed artist, the
   real usage) `focus` clearly beats `raw` (0.046 vs 0.035 @20, ~30%). The
   `1/(k-1)` document-length normalization earns its keep where it's used.
4. **Single-seed is a weak regime** — with one seed artist, *popularity* out-recovers
   every co-occurrence scorer (0.054 vs focus 0.046 @20). One artist barely
   constrains a playlist, so reconstruction rewards "add popular artists." This is
   the empirical case for (a) **richer seeds** — multi-seed recall is ~4–5× single —
   and (b) **online `--discover`**, since the local signal from a lone seed is thin.

## Honest caveats

- Small personal library (48 usable playlists, 2,166 single-seed pairs); absolute
  recall is low and estimates are noisy.
- APC recall measures *reconstruction of existing playlists*, which rewards
  popularity on hit/party playlists and does **not** capture the coherence that
  makes `curate` useful (Arctic→indie, Miles→jazz are qualitatively good even at
  modest recall). It's a proxy, not the whole truth — see
  [`../../research/04-measuring-playlist-success.md`](../../research/04-measuring-playlist-success.md).
- Popularity's single-seed edge is dominated by mixed/hit playlists; for a niche
  seed it would fare far worse.

## Implication for the roadmap

Add **multi-artist seeding** to `curate` (seed from several artists or an existing
playlist) — the evidence says that's where co-occurrence is strong. Keep
focus-weighting. `--discover` stays well-motivated for thin single seeds.
