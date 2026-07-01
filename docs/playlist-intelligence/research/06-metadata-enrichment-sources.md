# Metadata enrichment sources

_Part of the Playlist Intelligence research corpus for tapeit._

## Why this matters for us

tapeit knows a track only by **title / artist / album**, its **Apple Music catalog IDs**, and — often — its **ISRC**. It has **no audio files** on disk and **no access to Spotify's audio-features endpoint** (Spotify deprecated `audio_features`, `audio_analysis`, `recommendations`, related-artists and featured-playlists for all new Web API integrations on 27 Nov 2024, returning `403` with no waitlist or replacement [1][2]). That kills the single most common way apps used to get tempo / key / energy / danceability / valence.

So the practical question is narrow: **given only text metadata + an ISRC (and no audio), which public sources can we actually query, and what can they give us?**

The honest answer, verified below:

- **BPM + musical key** are reachable for free, by title/artist, from **GetSongBPM** (attribution required) and — per-track — from the **Deezer public API** (`bpm` field, no auth). These are the realistic wins.
- **Energy / danceability / valence / mood** (the Spotify-style 0-1 vectors) are **not freely available by metadata alone**. Every source that returns them either needs the audio (ReccoBeats upload, Essentia, librosa), needs a Spotify ID we don't have (ReccoBeats by-ID), is paid/enterprise (Cyanite, ACRCloud), or is scraping-only (Tunebat, Musicstax). The crowd-sourced open dataset that once covered this, **AcousticBrainz, is frozen and its site shut down**.
- **Lyrics** are reachable for free via **LRCLIB** (plain + synced, no auth) and, with a key and a preview-only free tier, **Musixmatch**. **Genius** gives metadata/URLs but not licensed full lyrics via API.
- **ISRC + MusicBrainz** are the glue that lets us normalise identity across all of these.

## The landscape

| Source | Provides | Query by | Access | Status / caveats |
|---|---|---|---|---|
| **GetSongBPM** (getsongbpm.com) | BPM, musical key, time signature | Title/artist search, or its own song ID | Free, API key required | Mandatory backlink to getsongbpm.com or account suspended [3]. API host `api.getsong.co` [3] |
| **Deezer public API** | Per-track `bpm`, `gain`, ISRC, metadata | Track ID (`/track/{id}`), search by title/artist, or `/track/isrc:{ISRC}` | Free, no auth for public endpoints | `bpm` returned on the single-track endpoint, not on album track-lists [4][5]. Values are crowd/label-supplied, sometimes 0/missing |
| **ReccoBeats** (reccobeats.com) | Spotify-style vector: acousticness, danceability, energy, instrumentalness, liveness, loudness, speechiness, tempo, valence [6][7] | `/v1/track/:id/audio-features` (ReccoBeats or Spotify ID) [7][8]; or **upload ≤30s audio** to extract [6] | Free | **No ISRC / title search documented** — single-track lookup needs a ReccoBeats ID; batch/recommendation accept Spotify IDs [8]. Reliability reported as inconsistent [2] |
| **AcousticBrainz** | Essentia low+high-level: BPM, key, plus mood/genre models | MusicBrainz Recording MBID | Free data dumps | **Frozen**: stopped submissions ~mid-2022, site shut down early 2023; ~7M recordings in final dumps, quality flagged as unreliable [9][10] |
| **Tunebat** | Key, BPM, Camelot, energy/danceability/happiness etc. | Title/artist (website) | No official public API | Third-party wrappers/scrapers only [11] |
| **Musicstax** | Key, BPM, energy/valence etc. | Title/artist (website) | No official public API found | Displays Spotify-derived features; scraping-only, unverified for programmatic use |
| **Cyanite.ai** | AI mood/genre/energy tags, similarity, per-segment | Audio upload (GraphQL API) | Free test tier; paid by catalog size | Commercial; needs audio; contact for pricing [12] |
| **ACRCloud** | Recognition + metadata/features | Audio fingerprint / audio | Paid (free trial) | Commercial; needs audio |
| **Essentia / librosa / musicnn** | Compute BPM, key, MFCC, mood models yourself | **Raw audio file** | Free / open-source | Requires the decoded audio — which tapeit does not have [13] |
| **Spotify audio-features / Echo Nest** | The original vector | Spotify ID | **Defunct for new apps** | Deprecated 27 Nov 2024 [1][2]; Echo Nest API shut 31 May 2016 |
| **LRCLIB** (lrclib.net) | Plain + synchronized (LRC) lyrics | title+artist+album+duration (`/api/get`), or `/api/search` | Free, no auth, no key | No rate limit; User-Agent encouraged [14] |
| **Musixmatch API** | Lyrics, synced lyrics, rich metadata | Track search, ISRC, title/artist | Freemium, key required | Free tier = **~30% preview snippet only**; full/synced/commercial need paid licence [15] |
| **Genius API** | Song metadata, annotations, page URLs | Search, song/artist ID | Free key | **Not licensed full lyrics** via API; lyrics are Genius legal property [16] |
| **lyrics.ovh / ChartLyrics** | Plain lyrics | title/artist | Free | Reported dead / unreliable [17] |
| **Happi.dev** | Lyrics search + music metadata | Search | Freemium key | Beta lyrics API, still active [17] |
| **MusicBrainz** | Canonical IDs, ISRC↔recording, relationships | ISRC, MBID, search | Free, ~1 req/s | Attribution expected; the identity backbone [18] |
| **AcoustID / Chromaprint** | Track identity from audio fingerprint → MBID | **Audio fingerprint** + duration | Free (non-commercial) | Needs audio to fingerprint [19] |

## Audio features (tempo / key / energy)

**GetSongBPM** is the most directly usable free BPM/key source for a metadata-only tool. The API (host `api.getsong.co`, key required after registering an email) exposes artist/song search plus BPM, musical key and time-signature lookups. It is free "for private, educational or commercial use," but a **visible backlink to getsongbpm.com is mandatory** — they state accounts are suspended without notice if the link is missing [3]. Coverage skews toward popular/DJ-relevant catalogue; obscure tracks may be absent.

**Deezer public API** is the quietly pragmatic option: Deezer exposes a `bpm` (and `gain`) field on the **single-track endpoint** (`/track/{id}`), it's JSON, and public read endpoints need **no authentication** [4][5]. Crucially for us, Deezer supports **ISRC lookup** via `/track/isrc:{ISRC}`, so our ISRCs map straight to a Deezer track and its `bpm`. Caveats: `bpm` is not included when you list an album's tracks (must hit the track directly) [4], and values are label/crowd supplied — some tracks report `0` or omit it. No `key` and no energy/valence from Deezer.

**ReccoBeats** is the most-hyped "Spotify audio-features replacement." Verified: it returns the full Spotify-style vector — acousticness, danceability, energy, instrumentalness, liveness, loudness, speechiness, tempo, valence [6][7]. Two ways in: (a) `/v1/track/:id/audio-features`, where a single-track lookup needs a **ReccoBeats ID** and only the batch/recommendation endpoints accept **Spotify IDs** [7][8]; or (b) **upload an audio clip** (≤30s, ≤5MB, MP3/OGG/WAV/AIFF) to the extraction endpoint [6]. **The catch for tapeit:** there is **no documented ISRC or title/artist search** — so with only metadata + ISRC and no audio, we can't address a track unless we first resolve a Spotify track ID (Spotify's *search* endpoint still works even though audio-features doesn't). Reliability is also reported as inconsistent by third parties [2]. Treat as "possible but fragile."

**AcousticBrainz** was the open, crowd-sourced Essentia dataset keyed to MusicBrainz MBIDs — exactly the shape we'd want. It is **dead for our purposes**: submissions stopped around mid-2022, the site was shut down in early 2023, and the final data dumps (~7M de-duplicated recordings) were explicitly flagged by MetaBrainz as not high-quality enough to be useful [9][10]. It survives only as static dumps you'd host yourself, MBID-keyed, with spotty coverage and no updates for anything released after ~2022.

**Tunebat / Musicstax** both *display* key, BPM, Camelot and energy-style numbers on their websites, but neither publishes a documented official public API — access is via third-party scrapers/wrappers [11], which is brittle and ToS-risky. **Cyanite.ai** (GraphQL, mood/energy/genre AI tags, per-15s-segment) and **ACRCloud** are genuine APIs but **commercial and audio-input based** — Cyanite offers a small free test tier then prices by catalogue size [12]. All three need either scraping or audio/paid access, so none is a clean fit.

**Self-compute (Essentia / librosa / musicnn)** is the "do it yourself" route: Essentia's music extractor computes BPM, key, loudness and, via its TensorFlow models, mood/genre; librosa and musicnn similarly. They're free and open-source and would give us everything — **but they all require the decoded audio signal as input** [13], which tapeit does not have. Not viable unless we start ingesting audio.

**Spotify audio-features / Echo Nest** — confirmed defunct for us. Spotify deprecated the endpoints on 27 Nov 2024 for any integration without pre-existing extended access [1][2]; The Echo Nest (whose tech these features descended from) had its own API shut down back on 31 May 2016.

## Lyrics

**LRCLIB** is the standout for a free tool: fully open, **no key, no auth, no rate limit**, serving both **plain and synchronized (LRC)** lyrics. Primary endpoint `GET /api/get?artist_name=…&track_name=…&album_name=…&duration=…` (it matches on duration within ~±2s); a fuzzier `/api/search` also exists. Including a `User-Agent` is encouraged [14]. Coverage is community-driven and strongest for popular tracks; misses on long-tail.

**Musixmatch** is the licensed, official option — richest catalogue and true synced lyrics — but the **free developer tier returns only a ~30% preview snippet** and caps calls; full lyrics, synced lyrics and any commercial use require a **paid licence** [15]. It supports ISRC lookup, which pairs well with our data, but budget for a licence before shipping a real lyrics feature.

**Genius** exposes a free API (api.genius.com) for search, song metadata, annotations and the web page URL — but **not licensed full lyrics**; Genius treats its lyrics as legal property and does not serve them via the API (people scrape the HTML, which is against ToS) [16]. Useful for metadata/links, not for the lyric text itself.

**lyrics.ovh** and **ChartLyrics** are legacy free plain-lyrics APIs now widely reported as **dead/unreliable** [17]. **Happi.dev** still offers a freemium lyrics search API (key required) [17] as a fallback.

## ID resolution glue

Everything above hinges on mapping our track to each provider's identity space. Two tools do the heavy lifting:

- **MusicBrainz** is the free, canonical registry. We can look up a recording **by ISRC** (`/ws/2/isrc/{ISRC}`) to get its MBID and canonical title/artist, and request ISRCs on a recording via `inc=isrcs` [18]. MBIDs are the key that historical **AcousticBrainz** dumps are indexed by, and MusicBrainz is also what **AcoustID** resolves fingerprints to. Rate limit ~1 request/second; attribution expected.
- **AcoustID + Chromaprint** identify a track from an **audio fingerprint** (+ duration) and map it to MusicBrainz MBIDs; free for non-commercial use with a client key [19]. **But fingerprinting requires the audio**, which tapeit lacks — so AcoustID is only relevant if/when we ingest audio. For a metadata-only tool, **ISRC → MusicBrainz** is the usable path; AcoustID is not.

The practical chain for tapeit: **Apple ISRC → MusicBrainz MBID / canonical text**, and separately **ISRC → Deezer track** (for `bpm`), plus **title/artist → GetSongBPM** (for BPM/key) and **title/artist → LRCLIB** (for lyrics).

## Recommended stack for tapeit

Given "text metadata + ISRC, no audio," use these three, in this order:

1. **Deezer public API (ISRC lookup) for BPM** — no auth, ISRC maps directly, JSON, zero attribution burden. Take `bpm` from the single-track endpoint; treat `0`/missing as "unknown." [4][5]
2. **GetSongBPM for BPM + musical key** — free with a key, adds the *key* Deezer lacks, and cross-checks BPM. Costs us a mandatory visible backlink to getsongbpm.com [3]. Query by title/artist.
3. **LRCLIB for lyrics** — free, no key, plain + synced, ISRC-independent (queries by title/artist/album/duration) [14]. Add **Musixmatch** later only if we accept its paid licence for full/synced lyrics.

Use **MusicBrainz (ISRC lookup)** underneath all of them to normalise title/artist and to dedupe, since GetSongBPM and LRCLIB match on fuzzy text [18].

**What stays impossible without audio (or a Spotify ID):** the Spotify-style **energy / danceability / valence / mood** vector. ReccoBeats *could* deliver it but only via a Spotify ID we'd have to resolve, or by uploading audio [6][8]; Cyanite/ACRCloud need audio and money [12]; Essentia/librosa need audio [13]; AcousticBrainz is frozen and thin [9][10]. If mood/energy is a hard requirement, the least-bad path is **resolve a Spotify track ID (via Spotify's still-working search) → ReccoBeats batch audio-features**, accepting its reliability caveats — otherwise this dimension is out of reach for a metadata-only, no-audio tool.

## Open questions

- Can we reliably resolve **ISRC → Spotify track ID** at scale (Spotify search still works) to feed ReccoBeats' batch audio-features, and is ReccoBeats' reliability good enough to depend on? [2][8]
- What is **Deezer's actual `bpm` coverage** across our real playlists (how often is it `0`/absent), and how does it compare to GetSongBPM for the same tracks?
- Is hosting the **AcousticBrainz static dumps** (MBID-keyed) worth it for older catalogue despite the quality warnings, purely as a fallback for key/BPM? [9][10]
- Does **GetSongBPM's licence/backlink** requirement fit tapeit's UI/distribution model, or does the mandatory link rule it out for some surfaces? [3]
- For lyrics at any real scale, is a **Musixmatch commercial licence** justified, or is LRCLIB coverage sufficient for our catalogue? [14][15]

## Sources

[1] Spotify for Developers — "Introducing some changes to our Web API" (27 Nov 2024 deprecation of audio-features, audio-analysis, recommendations, etc.) — https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api
[2] TechCrunch — "Spotify cuts developer access to several of its recommendation features" — https://techcrunch.com/2024/11/27/spotify-cuts-developer-access-to-several-of-its-recommendation-features/
[3] GetSongBPM — "API for webmasters" (free, API key, mandatory backlink, `api.getsong.co`) — https://getsongbpm.com/api
[4] Deezer for Developers — API documentation (public endpoints, no-auth reads) — https://developers.deezer.com/api
[5] deezer-python docs — Track resource (fields incl. `bpm`, `gain`, `isrc`) — https://deezer-python.readthedocs.io/en/stable/api_reference/resources/track.html
[6] ReccoBeats — Audio Feature Extraction (returns 9 features; audio upload ≤30s/≤5MB, MP3/OGG/WAV/AIFF) — https://reccobeats.com/docs/documentation/Analysis/audio-features-extraction
[7] ReccoBeats — "Get track's audio features" endpoint (`GET /v1/track/:id/audio-features`) — https://reccobeats.com/docs/apis/get-track-audio-features
[8] ReccoBeats — Resource ID (accepts ReccoBeats ID and Spotify ID; single lookup needs ReccoBeats ID) — https://reccobeats.com/docs/documentation/Resources/resource-id
[9] MetaBrainz Blog — "AcousticBrainz: Making a hard decision to end the project" (Feb 2022) — https://blog.metabrainz.org/2022/02/16/acousticbrainz-making-a-hard-decision-to-end-the-project/
[10] MetaBrainz Community — "AcousticBrainz submissions, data dumps, and next steps" (submissions off, ~7M-recording dumps) — https://community.metabrainz.org/t/acousticbrainz-submissions-data-dumps-and-next-steps/589843
[11] Tunebat — Music Metadata API page / third-party wrappers (no documented official public API) — https://tunebat.com/API
[12] Cyanite.ai — FAQ (GraphQL API, free test tier, mood tags, catalogue-based pricing) — https://cyanite.ai/faq/
[13] Essentia — Music extractor documentation (computes tempo/key/etc. from audio input) — https://essentia.upf.edu/streaming_extractor_music.html
[14] LRCLIB — API documentation (`/api/get`, `/api/search`, no auth, plain + synced) — https://lrclib.net/docs
[15] APIs.io — Musixmatch plans/pricing (freemium; free tier ~30% preview; paid for full/synced/commercial) — https://plans.apis.io/plans/musixmatch/musixmatch-plans-pricing/
[16] lyricsgenius docs — "How It Works" (Genius API returns metadata; lyrics are Genius legal property, not served via API) — https://lyricsgenius.readthedocs.io/en/stable/how_it_works.html
[17] Happi.dev — Lyrics Search API reference (active freemium; lyrics.ovh/ChartLyrics noted dead elsewhere) — https://happi.readme.io/reference/lyrics-search-api
[18] MusicBrainz — API documentation (ISRC lookup, `inc=isrcs`, ~1 req/s) — https://musicbrainz.org/doc/MusicBrainz_API
[19] AcoustID — Web Service (fingerprint + duration → MBID; free for non-commercial, client key) — https://acoustid.org/webservice
