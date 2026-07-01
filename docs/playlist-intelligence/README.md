# Playlist Intelligence

A research corpus and feature plan for building **great playlists from a user's
own saved playlists** (plus feedback signals like liked songs), in collaboration
with LLMs.

## Why this exists

A user's owned playlists reveal their **taste** — artists, genres, eras, moods,
and which songs they group together. But taste alone doesn't make a *great*
playlist. Great playlists also need:

- **Craft** — sequencing, an energy arc, and smooth transitions between tracks.
- **Context** — the occasion, activity, or daypart the playlist is for.
- **Novelty** — the right dose of discovery vs. familiarity.
- **A definition of success** — how we'd even know a generated playlist is good.

This corpus investigates those four external bodies of knowledge (curation,
transitions, DJ/radio selection, measurement) plus the modeling question of
inferring taste from a library — so that a later feature is grounded in how the
craft and the science actually work, not guesswork.

## Contents

| File | What it covers |
| ---- | -------------- |
| [`PLAN.md`](PLAN.md) | Feature vision, the signals we'd use, how we'd measure success, and the ADRs we'd need to write. Synthesizes the research below. |
| [`research/01-what-makes-a-successful-playlist.md`](research/01-what-makes-a-successful-playlist.md) | Curation principles: coherence, cohesion, sequencing, energy arc, length, diversity vs. consistency, editorial vs. algorithmic. |
| [`research/02-music-transitions-and-flow.md`](research/02-music-transitions-and-flow.md) | What makes a good track-to-track transition: harmonic/key mixing, tempo, energy continuity, avoiding jarring jumps. |
| [`research/03-dj-and-radio-track-selection.md`](research/03-dj-and-radio-track-selection.md) | How club DJs and radio programmers pick the next track: crowd reading, energy journeys, the radio "hot clock," dayparting, rotation. |
| [`research/04-measuring-playlist-success.md`](research/04-measuring-playlist-success.md) | Metrics & evaluation: completion/skip/save rates, retention, A/B testing, offline recommender eval, human/LLM judging. |
| [`research/05-inferring-taste-from-libraries.md`](research/05-inferring-taste-from-libraries.md) | Modeling taste from libraries: audio features, embeddings, collaborative filtering, sequence/session models, and the limits of preference-only. |
| [`research/06-metadata-enrichment-sources.md`](research/06-metadata-enrichment-sources.md) | Public/online APIs to enrich tracks with tempo, key, energy/mood, and lyrics — what we can actually query from metadata + ISRC (no audio). |
| [`research/SOURCES.md`](research/SOURCES.md) | Consolidated bibliography (~100 references). |
| [`lab/`](lab/) | **Data-science lab** — a local SQLite DB built from our real `playlists/` exports + Jupyter notebooks that make the research empirical (co-occurrence, taste, sequencing, evaluation, LLM ideas). See [`lab/README.md`](lab/README.md). |

## How to use it

1. Read the research docs to build a shared model of the domain.
2. `PLAN.md` turns that into a concrete, testable feature direction and a list of
   ADRs to write.
3. Feed any/all of these to an LLM as context when designing or reviewing the
   feature — each research doc ends with an **"Implications for the feature"**
   section written for exactly that purpose.

## Status

Research documents are **web-sourced syntheses** (see each doc's Sources section
and `research/SOURCES.md`); treat claims as informed-but-verify. `PLAN.md` is our
own synthesis and is a living document.
