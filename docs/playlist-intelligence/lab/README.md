# Playlist Intelligence — Lab

A hands-on data-science layer over our **own** playlist data. It turns the
`playlists/*.json` exports into a local SQLite database and provides Jupyter
notebooks to explore the theories from the [research](../research/) docs and
prototype ML/LLM ideas — on real data, not toy examples.

## What's here

| File | What it is |
| ---- | ---------- |
| `build_db.py` | Stdlib-only importer: `playlists/*.json` → `playlists.db`. Enriches with per-artist stats and track↔track / artist↔artist co-occurrence. |
| `playlists.db` | Generated SQLite DB (git-ignored — rebuild any time). The shared substrate for the notebooks. |
| `requirements.txt` | Python deps for the notebooks (pandas, jupyter, etc.). `build_db.py` itself needs none. |
| `notebooks/01-library-eda.ipynb` | Explore the library: sizes, top artists, most-shared tracks. What taste looks like as data. |
| `notebooks/02-cooccurrence-and-taste.ipynb` | Co-occurrence → "songs/artists that go together"; PMI, similarity, a path toward embeddings & Automatic Playlist Continuation. |
| `notebooks/03-flow-eval-and-llm-ideas.ipynb` | Sequencing/flow with metadata only (no audio features), an offline APC-style holdout evaluation, and where an LLM fits. |

## Setup

```bash
# 1. Build the database from the committed playlist exports (no deps):
python3 docs/playlist-intelligence/lab/build_db.py

# 2. Install notebook deps and launch:
python3 -m venv .venv && source .venv/bin/activate
pip install -r docs/playlist-intelligence/lab/requirements.txt
jupyter lab docs/playlist-intelligence/lab/notebooks/
```

The database rebuilds deterministically from the JSON, so re-run `build_db.py`
after `tapeit import`/`create` changes the playlists.

## Schema

```
playlists(id, slug, name, track_count, cooccurrence_included)
tracks(id, key, title, artist, album, isrc, apple_id)   -- key = norm(title)|norm(artist)
playlist_tracks(playlist_id, track_id, position)        -- ordered membership
track_cooccurrence(track_a, track_b, weight)            -- pairs within a playlist
artist_cooccurrence(artist_a, artist_b, weight)
```

`cooccurrence_included = 0` marks playlists over ~250 tracks (e.g. Liked-Songs
dumps): their tracks are imported but their pairs are excluded from the
co-occurrence tables, so intentional grouping isn't drowned out. See `build_db.py`.

## Caveat: no rich audio features

Apple Music / MusicKit exposes only title/artist/album-style metadata, and
Spotify restricted its `audio_features` endpoint for new apps in Nov 2024 (see
[research/02](../research/02-music-transitions-and-flow.md) and
[research/05](../research/05-inferring-taste-from-libraries.md)). So the lab
works from **co-occurrence and metadata**, not tempo/key/energy — a real
constraint the notebooks are honest about and design around.
