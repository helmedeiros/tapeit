"""Build a local SQLite database from the tapeit `playlists/*.json` exports.

The database is the shared substrate for the Playlist Intelligence notebooks:
import the playlists we own, enrich them (per-artist stats, track↔track and
artist↔artist co-occurrence), and explore taste/curation questions with plain
SQL or pandas.

Stdlib only — no third-party dependencies. Regenerate any time:

    python3 docs/playlist-intelligence/lab/build_db.py

Output: docs/playlist-intelligence/lab/playlists.db (git-ignored; rebuildable).
"""

from __future__ import annotations

import json
import re
import sqlite3
import sys
import unicodedata
from itertools import combinations
from pathlib import Path

# Playlists larger than this are skipped for the pairwise co-occurrence table:
# a 1,000-track "Liked Songs" dump would emit ~500k pairs and dominate the
# signal without saying much about intentional grouping. Their tracks are still
# imported; only their pairs are omitted (noted in playlist.cooccurrence_included).
COOCCURRENCE_MAX_PLAYLIST = 250

REPO_ROOT = Path(__file__).resolve().parents[3]
PLAYLISTS_DIR = REPO_ROOT / "playlists"
DB_PATH = Path(__file__).resolve().parent / "playlists.db"


def norm(s: str) -> str:
    s = unicodedata.normalize("NFKD", s or "").lower()
    s = "".join(c for c in s if not unicodedata.combining(c))
    return re.sub(r"\s+", " ", re.sub(r"[^0-9a-z\s]", " ", s)).strip()


def load_playlists() -> list[dict]:
    out = []
    for path in sorted(PLAYLISTS_DIR.glob("*.json")):
        with path.open() as f:
            doc = json.load(f)
        out.append({"slug": path.stem, "name": doc.get("name", path.stem), "tracks": doc.get("tracks", [])})
    return out


def schema(db: sqlite3.Connection) -> None:
    db.executescript(
        """
        DROP TABLE IF EXISTS playlists;
        DROP TABLE IF EXISTS tracks;
        DROP TABLE IF EXISTS playlist_tracks;
        DROP TABLE IF EXISTS track_cooccurrence;
        DROP TABLE IF EXISTS artist_cooccurrence;

        CREATE TABLE playlists (
            id INTEGER PRIMARY KEY,
            slug TEXT UNIQUE,
            name TEXT,
            track_count INTEGER,
            cooccurrence_included INTEGER   -- 1 if its pairs fed the co-occurrence tables
        );
        CREATE TABLE tracks (
            id INTEGER PRIMARY KEY,
            key TEXT UNIQUE,                -- norm(title)|norm(artist)
            title TEXT,
            artist TEXT,
            album TEXT,
            isrc TEXT,
            apple_id TEXT
        );
        CREATE TABLE playlist_tracks (
            playlist_id INTEGER,
            track_id INTEGER,
            position INTEGER,
            PRIMARY KEY (playlist_id, position)
        );
        CREATE TABLE track_cooccurrence (
            track_a INTEGER, track_b INTEGER, weight INTEGER,
            PRIMARY KEY (track_a, track_b)
        );
        CREATE TABLE artist_cooccurrence (
            artist_a TEXT, artist_b TEXT, weight INTEGER,
            PRIMARY KEY (artist_a, artist_b)
        );
        """
    )


def build() -> None:
    if not PLAYLISTS_DIR.is_dir():
        sys.exit(f"no playlists dir at {PLAYLISTS_DIR}")
    playlists = load_playlists()

    db = sqlite3.connect(DB_PATH)
    schema(db)

    track_id: dict[str, int] = {}
    track_pairs: dict[tuple[int, int], int] = {}
    artist_pairs: dict[tuple[str, str], int] = {}

    for pl in playlists:
        included = len(pl["tracks"]) <= COOCCURRENCE_MAX_PLAYLIST
        pid = db.execute(
            "INSERT INTO playlists (slug, name, track_count, cooccurrence_included) VALUES (?,?,?,?)",
            (pl["slug"], pl["name"], len(pl["tracks"]), int(included)),
        ).lastrowid

        ids_in_pl: list[int] = []
        artists_in_pl: set[str] = set()
        for pos, t in enumerate(pl["tracks"]):
            title, artist = t.get("title", ""), t.get("artist", "")
            key = f"{norm(title)}|{norm(artist)}"
            tid = track_id.get(key)
            if tid is None:
                ids = t.get("ids") or {}
                tid = db.execute(
                    "INSERT INTO tracks (key, title, artist, album, isrc, apple_id) VALUES (?,?,?,?,?,?)",
                    (key, title, artist, t.get("album"), t.get("isrc"), ids.get("appleMusic")),
                ).lastrowid
                track_id[key] = tid
            db.execute(
                "INSERT INTO playlist_tracks (playlist_id, track_id, position) VALUES (?,?,?)",
                (pid, tid, pos),
            )
            ids_in_pl.append(tid)
            if norm(artist):
                artists_in_pl.add(artist)

        if included:
            for a, b in combinations(sorted(set(ids_in_pl)), 2):
                track_pairs[(a, b)] = track_pairs.get((a, b), 0) + 1
            for a, b in combinations(sorted(artists_in_pl), 2):
                artist_pairs[(a, b)] = artist_pairs.get((a, b), 0) + 1

    db.executemany(
        "INSERT INTO track_cooccurrence (track_a, track_b, weight) VALUES (?,?,?)",
        [(a, b, w) for (a, b), w in track_pairs.items()],
    )
    db.executemany(
        "INSERT INTO artist_cooccurrence (artist_a, artist_b, weight) VALUES (?,?,?)",
        [(a, b, w) for (a, b), w in artist_pairs.items()],
    )
    db.commit()

    n_pl = db.execute("SELECT COUNT(*) FROM playlists").fetchone()[0]
    n_tr = db.execute("SELECT COUNT(*) FROM tracks").fetchone()[0]
    n_rows = db.execute("SELECT COUNT(*) FROM playlist_tracks").fetchone()[0]
    n_pairs = db.execute("SELECT COUNT(*) FROM track_cooccurrence").fetchone()[0]
    print(f"✓ {DB_PATH.name}: {n_pl} playlists, {n_tr} unique tracks, "
          f"{n_rows} memberships, {n_pairs} track co-occurrence pairs")
    db.close()


if __name__ == "__main__":
    build()
