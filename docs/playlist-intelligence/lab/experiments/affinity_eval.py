"""Leave-one-out APC-style evaluation of artist-affinity scorers.

Empirically tests the candidate-generation heuristics used by `tapeit curate`
against the real library: for each multi-artist playlist, hide its later
artists, seed from the earlier ones, and measure how well each scorer recovers
the held-out artists — the Automatic Playlist Continuation protocol
(RecSys 2018 / Million Playlist Dataset) applied at the artist level.

Scorers compared:
  popularity   – rank by how many playlists an artist appears in (baseline)
  raw          – sum of shared-playlist counts with the seed artists
  focus        – shared playlists weighted 1/(k-1) per k-artist playlist (curate today)
  pmi          – summed pointwise mutual information (unsmoothed)
  pmi_gated    – PMI but only over pairs co-occurring >= 2 times (popularity-robust)

Stdlib only. Run:  python3 docs/playlist-intelligence/lab/experiments/affinity_eval.py
"""

from __future__ import annotations

import math
import sqlite3
from collections import defaultdict
from pathlib import Path

DB = Path(__file__).resolve().parents[1] / "playlists.db"
COOC_MAX = 250   # match curate: ignore big "dump" playlists for co-occurrence
MIN_ARTISTS = 8  # only test playlists with enough distinct artists
HOLDOUT = 0.4    # fraction of a playlist's artists (the later ones) held out
KS = (10, 20)


def playlist_artists(db) -> dict[int, list[str]]:
    """Distinct artists per playlist, in first-appearance order."""
    rows = db.execute(
        """SELECT pt.playlist_id, t.artist, MIN(pt.position) pos
           FROM playlist_tracks pt JOIN tracks t ON t.id = pt.track_id
           WHERE t.artist <> '' GROUP BY pt.playlist_id, t.artist"""
    ).fetchall()
    by_pl = defaultdict(list)
    for pid, artist, pos in rows:
        by_pl[pid].append((pos, artist))
    return {pid: [a for _, a in sorted(v)] for pid, v in by_pl.items()}


def build_global(artists_by_pl, sizes):
    """Global co-occurrence over playlists <= COOC_MAX tracks."""
    pairs = defaultdict(lambda: defaultdict(int))
    wpairs = defaultdict(lambda: defaultdict(float))
    freq = defaultdict(int)
    counted = []
    for pid, arts in artists_by_pl.items():
        if sizes[pid] > COOC_MAX or len(arts) < 2:
            continue
        counted.append(pid)
        share = 1.0 / (len(arts) - 1)
        for a in arts:
            freq[a] += 1
        for i in range(len(arts)):
            for j in range(i + 1, len(arts)):
                a, b = arts[i], arts[j]
                pairs[a][b] += 1; pairs[b][a] += 1
                wpairs[a][b] += share; wpairs[b][a] += share
    return pairs, wpairs, freq, len(counted)


def score_candidates(strategy, seed, pairs, wpairs, freq, N, drop):
    """Return {candidate: score} for one test playlist under a strategy.

    `drop(a,b)` gives the leave-one-out adjustment to remove the test playlist's
    own contribution to a pair; `dfreq(a)` / N-1 handle the same for frequency.
    """
    cand = defaultdict(float)
    for s in seed:
        fs = freq[s] - drop["freq"](s)
        if fs <= 0:
            continue
        for c, raw in pairs[s].items():
            if c in seed:
                continue
            co = raw - drop["pair"](s, c)          # LOO co-occurrence count
            if co <= 0:
                continue
            fc = freq[c] - drop["freq"](c)
            if fc <= 0:
                continue
            if strategy == "raw":
                cand[c] += co
            elif strategy == "focus":
                cand[c] += wpairs[s][c] - drop["wpair"](s, c)
            elif strategy == "pmi":
                cand[c] += math.log(co * (N - 1) / (fs * fc))
            elif strategy == "pmi_gated":
                if co >= 2:
                    cand[c] += math.log(co * (N - 1) / (fs * fc))
    return cand


def popularity(seed, freq, drop):
    return {a: freq[a] - drop["freq"](a) for a in freq if a not in seed}


def evaluate():
    db = sqlite3.connect(DB)
    sizes = dict(db.execute("SELECT id, track_count FROM playlists").fetchall())
    artists_by_pl = playlist_artists(db)
    pairs, wpairs, freq, N = build_global(artists_by_pl, sizes)

    strategies = ["popularity", "raw", "focus", "pmi", "pmi_gated"]
    recall = {s: {k: [] for k in KS} for s in strategies}
    rprec = {s: [] for s in strategies}
    single = {s: {k: [] for k in KS} for s in strategies}  # curate's single-seed use
    n_tests = n_single = 0

    for pid, arts in artists_by_pl.items():
        if sizes[pid] > COOC_MAX or len(arts) < MIN_ARTISTS:
            continue
        cut = int(round(len(arts) * (1 - HOLDOUT)))
        seed, held = set(arts[:cut]), arts[cut:]
        pset = set(arts)
        in_pl = lambda a: 1 if a in pset else 0
        both = lambda a, b: 1 if a in pset and b in pset else 0
        share_p = 1.0 / (len(arts) - 1)
        drop = {"freq": in_pl,
                "pair": lambda a, b: both(a, b),
                "wpair": lambda a, b: share_p * both(a, b)}
        # recoverable held-out artists: those present in at least one OTHER playlist
        held_rec = [a for a in held if freq[a] - in_pl(a) >= 1]
        if not held_rec:
            continue
        n_tests += 1
        held_set = set(held_rec)

        for s in strategies:
            cand = (popularity(seed, freq, drop) if s == "popularity"
                    else score_candidates(s, seed, pairs, wpairs, freq, N, drop))
            ranked = [a for a, _ in sorted(cand.items(), key=lambda kv: (-kv[1], kv[0]))]
            for k in KS:
                topk = set(ranked[:k])
                recall[s][k].append(len(held_set & topk) / len(held_set))
            r = len(held_rec)
            rprec[s].append(len(held_set & set(ranked[:r])) / r)

        # Single-seed mode: mirrors `tapeit curate --seed <one artist>`.
        for s0 in arts:
            held1 = {a for a in arts if a != s0 and freq[a] - in_pl(a) >= 1}
            if not held1:
                continue
            n_single += 1
            for s in strategies:
                seed1 = {s0}
                cand = (popularity(seed1, freq, drop) if s == "popularity"
                        else score_candidates(s, seed1, pairs, wpairs, freq, N, drop))
                ranked = [a for a, _ in sorted(cand.items(), key=lambda kv: (-kv[1], kv[0]))]
                for k in KS:
                    single[s][k].append(len(held1 & set(ranked[:k])) / len(held1))

    print(f"Leave-one-out APC eval — {n_tests} multi-artist playlists "
          f"(>= {MIN_ARTISTS} artists, <= {COOC_MAX} tracks), holdout={HOLDOUT}\n")
    hdr = f"{'strategy':<12}" + "".join(f" Recall@{k:<4}" for k in KS) + " R-precision"
    print(hdr); print("-" * len(hdr))
    for s in strategies:
        row = f"{s:<12}"
        for k in KS:
            row += f"  {sum(recall[s][k]) / n_tests:6.3f} "
        row += f"    {sum(rprec[s]) / n_tests:6.3f}"
        print(row)

    def head_to_head(a, b, k=20):
        w = t = l = 0
        for ra, rb in zip(recall[a][k], recall[b][k]):
            w += ra > rb; t += ra == rb; l += ra < rb
        print(f"  {a} vs {b} @Recall{k}:  {w} wins / {t} ties / {l} losses")

    print("\nPer-playlist head-to-head (multi-seed):")
    head_to_head("focus", "raw")
    head_to_head("focus", "pmi_gated")
    head_to_head("raw", "pmi_gated")

    print(f"\nSingle-seed eval — {n_single} (playlist, seed-artist) pairs "
          "(mirrors `curate --seed`)\n")
    print(hdr); print("-" * len(hdr))
    for s in strategies:
        row = f"{s:<12}"
        for k in KS:
            row += f"  {sum(single[s][k]) / n_single:6.3f} "
        print(row + "       —")


if __name__ == "__main__":
    evaluate()
