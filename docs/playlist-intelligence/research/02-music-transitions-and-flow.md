# Music transitions and flow

_Part of the Playlist Intelligence research corpus._

## Why this matters for us

We are building a feature that **sequences a playlist from a user's own saved playlists / library** — reorder (and possibly select) tracks so the result plays well front-to-back. Critically, we deal with **listening playlists**, not live DJ sets: every track plays start-to-finish, and we do not beatmatch or crossfade. "Flow" for us therefore means *adjacency quality* — does each track feel like a reasonable neighbour of the one before it — plus a sensible **arc** across the whole list (energy build/release), rather than seamless mid-track blends.

Two consequences fall out of this immediately:
- The DJ literature (harmonic mixing, BPM matching, energy levels) gives us a well-developed vocabulary and rules of thumb, but many of its rules exist to make *overlapping* audio blend. When tracks don't overlap, key clashes and small BPM gaps matter far less; **mood, energy, and timbre continuity matter more** [6][7].
- The audio features that would let us score transitions programmatically (key, tempo, energy, valence, loudness) are **easy to get on Spotify historically but now deprecated, and largely absent from Apple Music** [8][9]. This constrains implementation and is covered under Implications.

## Key findings

### Harmonic mixing and key compatibility (Camelot wheel)
- DJs use the **Camelot wheel**, a relabelling of the circle of fifths. Each of the 24 major/minor keys gets a code: a number 1–12 plus a letter (**A = minor, B = major**) [1][2].
- The four "safe" moves that sound smooth are: **same key** (e.g. 8A→8A); **±1 on the same letter** (8A→7A or 9A), i.e. adjacent on the wheel; **relative major/minor** (same number, switch letter, 8A→8B), which shifts mood darker↔brighter without clashing; and the intentional **+2 "energy boost"** used for lift [1][3]. Mixed In Key confirms the first three as the core compatible moves and notes big jumps (e.g. 3A→9A) are "brazen" curveballs used deliberately, not by default [3].
- Larger, arbitrary key jumps are the ones that "clash" when audio overlaps. Notably, the sources stress the wheel is **a guideline, not a law** — a genre change or a hard cut can hide a key jump because the sonic contrast is expected [1].

### Tempo (BPM)
- For DJ beatmatching, tracks are kept within a **~5 BPM** window of each other so they can be sped/slowed to align without artifacts; sorting a set into BPM ranges is standard practice [4][5].
- Large BPM changes are **jarring in a beatmatched context** because time-stretching audio by a large percentage produces "glitchy" / distorted artifacts, and one track ends up sounding obviously too fast or slow [4]. Advanced DJs only bridge big gaps with special techniques (loops, echo-outs, cutting on a breakdown) [4].
- For **listening playlists** the same ~5 BPM adjacency heuristic is repeated by curation guides as a way to make lists "flow naturally, avoiding jarring shifts" [6] — but here the reason is *perceived pace continuity*, not the mechanical need to align beats. A tempo step feels smoother than a tempo cliff.

### Energy and intensity continuity
- Mixed In Key rates every track on an **Energy Level 1–10** (1 = ambient/no beat, 5 = dancing begins, 7 = high-energy party, 8+ = festival "hands-in-the-air" anthems) and recommends moving **one step at a time**; skipping levels (e.g. 5→7) "sounds like a different DJ got on the decks" [7].
- Good sets/playlists follow an **intentional energy arc** rather than a flat line — build up, sustain, and use deliberate **dips** (e.g. an Energy 4 track after a run of anthems) to let listeners "cool down" before rebuilding [7][10].
- Album-sequencing craft frames this as a **rollercoaster**: "moments of intensity followed by periods of calm" keep listeners engaged; a string of moody tracks is often followed by something uplifting to release tension [11]. The failure mode to avoid is an **unmotivated sudden drop or spike** in energy between neighbours.

### Timbre, instrumentation, and mood continuity
- Curation guides advise grouping by **similar tempo, key, energy, vocals, and genre/mood** so adjacent tracks share a sonic palette [6][7]. When you can't crossfade, matching *instrumentation and overall vibe* is what makes one song feel like it belongs next to another.
- Album sequencing explicitly lists **tempo, key, energy, mood, and the tracks' intros/outros** as the elements to weigh when ordering — i.e. how a song *ends* and how the next one *begins* matters even without a crossfade [11].

### DJ beatmatched transitions vs. listening-playlist transitions
- In a DJ set, tracks **overlap**; the transition is an engineered event (beatmatch + crossfade + EQ) where key clashes and BPM mismatch produce audible dissonance. Harmonic mixing and ±5 BPM exist largely to serve that overlap [1][4].
- In a **listening playlist**, "tracks are played in their entirety with little or no thought given to their ordering" — Spotify Research frames improving this as sequencing (which order) separately from transitions (optional DJ-style crossfades), and notes average listeners lack curator expertise [12]. For us, only the **sequencing** half applies: flow = a good *ordering* so consecutive full tracks feel continuous in pace, energy, and mood, plus a coherent whole-list arc. Silence/gap between tracks even acts as a natural "reset," reducing the penalty for a moderate shift [11].

### Measurable audio features for scoring transitions
- A recent peer-reviewed study (Schweiger, Parada-Cabaleiro, Schedl, *EPJ Data Science* 2025) gives a **formal definition of playlist coherence based on sequential ordering** — "smooth transitions between tracks" — across **ten auditory features plus one metadata feature**, on a large corpus of user-curated playlists [13]. Headline results: **longer playlists are more coherent**, while playlists **dominated by popular tracks** or **heavily edited by users** are *less* coherent [13]. (We could not extract the per-feature coherence rankings from the open abstract; treat the specific feature weighting as **uncertain** pending the full paper.)
- The features historically used for exactly this — **key, mode, tempo (BPM), energy, valence, loudness, danceability, acousticness, instrumentalness** — came from Spotify's `audio_features` endpoint (a 13-field vector per track) [8]. This is the concrete, well-validated feature set the field relied on for mood/energy sequencing.

### Audio-feature availability (Apple/iTunes vs. Spotify)
- **Spotify deprecated `audio_features`, `audio_analysis`, recommendations, and related endpoints on 27 Nov 2024**; new apps get 403s, and apps that built mood/energy/workout sequencing on these fields "broke completely," with no official replacement 18 months on [8].
- **Apple Music / MusicKit does not expose per-track BPM, key, or tempo** for catalog tracks; developers have repeatedly flagged this as missing, and advanced audio analysis (beatmatching-grade) is limited to approved DJ partners (djay, Serato, rekordbox, Engine) [9]. One comparison notes Apple is missing roughly six of Spotify's eleven analytic fields (danceability, energy, valence, acousticness, instrumentalness, liveness) [8].
- Practical implication: to score transitions on Apple Music content we'd need a **third-party or self-computed feature source** (e.g. AcousticBrainz / Essentia-style analysis, or a BPM/key service) [8][9].

## Principles we can operationalize

Assume for each track a feature vector `{tempo, key(Camelot), energy, valence, loudness, mood/genre tags}`. Score each **adjacent pair** and sum/penalize across the list.

1. **Tempo step, not cliff.** Reward `|ΔBPM| ≤ ~5` between neighbours; apply a soft, increasing penalty beyond that. Treat half/double-time as compatible (e.g. 70 vs 140) [4][6].
2. **Key adjacency (soft, low weight for listening).** Reward Camelot moves in {same, ±1 same letter, same-number A↔B, +2}; mild penalty otherwise. Weight this **lower than energy/mood** because tracks don't overlap [1][3][7].
3. **Energy continuity with intentional arc.** Penalize large `|ΔEnergy|` between neighbours (the "one step at a time" rule), but score the *whole list* against a target **energy curve** (e.g. gentle build then release) rather than forcing a flat line — allow deliberate dips [7][10][11].
4. **Mood/valence continuity.** Penalize big valence swings between neighbours (e.g. euphoric → bleak) unless a section boundary justifies it [11][13].
5. **Timbre/genre cohesion.** Reward shared genre/instrumentation tags between neighbours; a genre change is allowed but should coincide with an intended section break (it can even *mask* a key/BPM jump) [1][6].
6. **Loudness leveling.** Prefer neighbours with similar loudness to avoid a jarring volume jump [8][13].
7. **Combined adjacency score** = weighted sum of the above, with **energy + mood + tempo weighted highest** and **key + loudness lower**, reflecting that we sequence full tracks, not crossfades. Then optimize the ordering (Spotify Research models sequencing as **graph traversal** — nodes = tracks, edge weight = transition quality, find a good path) [12].

## Implications for the feature

- **Model it as ordering, not blending.** Build a graph over the user's candidate tracks, weight edges by the adjacency score above, and find a high-quality path (optionally seeded/anchored, and optionally shaped to a target energy curve). This matches how Spotify Research frames automatic sequencing [12] and how the coherence literature defines the goal [13].
- **Feature acquisition is the real constraint.**
  - If sourcing from **Spotify**: the ideal features (energy, valence, tempo, key, loudness, danceability) existed but are **deprecated as of Nov 2024** — do not assume `audio_features` is available for new API access [8].
  - If sourcing from **Apple Music** (our current stack, per repo): MusicKit gives catalog metadata but **no BPM/key/energy/valence** [9]. We would need a **third-party analysis or feature service**, or compute features ourselves from audio, to run the scoring rules above [8][9].
  - **Fallback with metadata only:** if audio features are unavailable, approximate flow from **genre, release era, artist, and any available BPM/mood tags**, plus co-occurrence signals (tracks users already place near each other) — the hybrid text+audio and co-occurrence approaches from the APC literature degrade gracefully when audio features are missing [14].
- **Don't over-index on harmonic mixing.** Because we never overlap tracks, key compatibility should be a **tie-breaker, not a gate**. Energy/mood/tempo continuity and the overall arc are what listeners actually perceive across full-track playback [6][7][11].
- **Whole-list arc as a first-class objective.** Offer curve presets (steady focus, build-up, wind-down/sleep, workout ramp) since the "right" energy shape is context-dependent — the same finding recurs in DJ, streaming, and album-sequencing sources [7][10][11].

## Open questions

- **Which features dominate perceived coherence?** The EPJ 2025 study quantifies coherence across ten features but we couldn't extract the per-feature ranking from the open text — need the full paper to weight our score empirically [13].
- **How much does key really matter without crossfades?** All harmonic-mixing evidence is from overlapping DJ contexts; we have no direct evidence on whether listeners notice key adjacency between fully-separated tracks. Likely low, but unvalidated.
- **Best practical feature source for Apple Music.** Which third-party service (or self-hosted Essentia/AcousticBrainz-style pipeline) gives reliable BPM/key/energy at catalog scale, and at what cost/latency? [8][9]
- **Optimal energy-curve shapes per use case**, and whether to infer the intended curve from the user's *existing* playlist orderings rather than imposing a preset.
- **Half/double-time and section boundaries** — how to detect a legitimate "reset" point where a bigger jump is acceptable (album sequencing treats gaps/silence as resets [11]).

## Sources

[1] The DJ's Guide to the Camelot Wheel and Harmonic Mixing — https://dj.studio/blog/camelot-wheel — Explains Camelot wheel as circle-of-fifths relabel, the safe moves, and that it's a guideline not a law.
[2] Camelot Wheel Reference: All 24 Codes & Compatibility Rules — https://vibesdj.io/learn/techniques/camelot-wheel — A/B (minor/major) numbering and compatibility rules.
[3] Harmonic Mixing Guide — Mixed In Key — https://mixedinkey.com/harmonic-mixing-guide/ — Same key, ±1, and relative major/minor as core compatible moves; curveball jumps as risky.
[4] A Beginner's Guide to Beat Matching for DJs — Point Blank — https://www.pointblankmusicschool.com/blog/a-beginners-guide-to-beat-matching-for-djs/ — Why large BPM gaps distort when time-stretched; keeping tracks in similar BPM ranges.
[5] How To Beat Match: Step-by-Step Guide — Learningtodj.com — https://learningtodj.com/blog/how-to-beat-match-step-by-step-guide-for-new-djs/ — Sorting tracks into ~5 BPM ranges for smoother beatmatching.
[6] Playlist Sounds: How to Curate Playlists That Flow Seamlessly — Musosoup — https://musosoup.com/blog/playlist-sounds — ~5 BPM adjacency and grouping by tempo/energy for listening-playlist flow.
[7] Sorting your playlists by Energy Level — Mixed In Key — https://mixedinkey.com/harmonic-mixing-guide/sorting-playlists-by-energy-level/ — Energy Level 1–10 definitions and the "one step at a time" progression / strategic dips.
[8] Spotify Audio Features Is Dead — FreqBlog — https://freqblog.com/blog/spotify-audio-features-replacement-2026/ — Nov 27 2024 deprecation, the 13-field vector, and Apple Music's missing fields.
[9] BPM/Tempo information for Songs via Apple Music API — Apple Developer Forums — https://developer.apple.com/forums/thread/726626 — Confirms Apple Music API lacks per-track BPM/key/tempo; advanced analysis limited to partners.
[10] Controlling the dancefloor: organizing playlists by energy — DJ TechTools — https://djtechtools.com/2022/11/25/controlling-the-dancefloor-a-guide-on-organizing-playlists-by-energy/ — Energy tiers, sorting by BPM then key/energy, and intentional progression.
[11] Album Sequencing: 6 Ways to Give Your Tracklist That Perfect Flow — LANDR — https://blog.landr.com/album-sequencing/ — Tension/release arc, weighing tempo/key/energy/mood and intros/outros, silence as a reset.
[12] Automatic Playlist Sequencing and Transitions — Spotify Research — https://research.atspotify.com/publications/automatic-playlist-sequencing-and-transitions — Sequencing as graph traversal, transitions as optimization; separates ordering from crossfades.
[13] The impact of playlist characteristics on coherence in user-curated music playlists — EPJ Data Science (2025) — https://link.springer.com/article/10.1140/epjds/s13688-025-00531-3 — Formal coherence metric on sequential ordering over ten auditory features; longer playlists more coherent, popular/edited less.
[14] Automatic playlist continuation using a hybrid recommender combining text and audio — arXiv 1901.00450 — https://arxiv.org/abs/1901.00450 — Hybrid audio+title+co-occurrence approach to extending playlists while preserving characteristics.
