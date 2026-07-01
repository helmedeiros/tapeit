# Inferring taste from libraries

_Part of the Playlist Intelligence corpus._

## Why this matters for us

Our raw material is what the user already owns: their saved/exported playlists plus
liked songs. From TapeIt's JSON exports we get **title / artist / album** per track and
the grouping of tracks into named playlists — and, critically, we do **not** get rich
audio features from Apple Music. So the practical question is: how much of a user's
taste can we reconstruct from co-occurrence and metadata alone, and where does a
preference-only view break down? This note surveys the recommender-systems literature
on inferring taste from libraries and the well-documented limits of over-fitting to
existing preferences, so we build candidate generation that is both faithful to the
user and deliberately not a filter bubble.

## Key findings

**1. Playlist co-occurrence is a first-class taste signal — often stronger than raw plays.**
The core insight behind modern music recommenders is "organizational similarity":
two songs are similar if users tend to put them on the same playlist. Spotify reportedly
trains on hundreds of millions of user-generated playlists precisely because manual
curation signals deeper affinity than passive stream counts [12]. This is exactly the
signal we already hold in exported playlists — the user themselves has grouped songs.

**2. Automatic Playlist Continuation (APC) is the canonical framing for "extend what you already have."**
The ACM RecSys Challenge 2018, built on Spotify's Million Playlist Dataset (1M real
playlists), defined the task as: given a playlist's title and/or a subset of its tracks,
recommend up to 500 tracks that fit [3]. This maps directly onto our use case ("build/
extend from the user's library"). The challenge drew 113 teams; the winning main-track
entry reached R-precision ~0.224 and NDCG ~0.395 [3] — a useful reminder that even
state-of-the-art APC is far from "solved," so we should present candidates, not verdicts.

**3. Neighborhood collaborative filtering is a strong, simple baseline for APC.**
Top challenge solutions were often not deep nets: a composition of collaborative filters,
each capturing a different aspect of a playlist, with the combination tuned via a
Tree-structured Parzen Estimator, was highly competitive [4]. Broader analyses found that
simple neighbor-based methods with list-song similarity functions performed near the top
[3][5]. Encouraging for us: co-occurrence neighborhoods are computable from metadata alone.

**4. Track/playlist embeddings (word2vec-style) capture "songs that go together" without audio.**
Treat a playlist (or listening session) as a *sentence* and each song as a *word*; train
skip-gram so each song predicts its neighbors. Songs appearing in similar contexts end up
close in vector space [6]. A user's taste can then be represented by **averaging the
vectors of their songs**, and neighbors of that centroid become recommendations — the
Anghami example recommended indie-folk tracks (Fleet Foxes, Ben Howard) from an indie-folk
seed set purely via co-occurrence geometry, no audio analysis [6]. This is content-free
and depends only on grouping, which is what our exports contain.

**5. Session/sequence models add ordering, but ordering is a weaker signal for owned playlists.**
GRU4Rec introduced RNNs for session-based next-item prediction; SASRec and BERT4Rec then
showed self-attention/transformers capture long-range sequence dependencies better than
RNNs/CNNs [10]. Transformers4Rec explicitly bridges NLP sequence modeling and session
recommendation [10]. Caveat for us: these shine on *temporal* sessions (skips/plays in
order). A saved playlist's track order is often intentional (sequencing) but sometimes
arbitrary, so we should treat sequence as a soft signal, not ground truth.

**6. Content-based, collaborative, and hybrid each cover the other's blind spots.**
Content-based models describe *how a track sounds* independent of behavior (audio features
like danceability/energy/valence, plus metadata and, increasingly, LLM embeddings of
lyrics/cover art); collaborative filtering describes *who groups it with what*; production
systems (e.g. Spotify) hybridize all three plus user-taste embeddings [12]. Hybridization
also mitigates **cold-start**: new tracks lean on content features until collaborative
signal accumulates [12]. Our constraint is that the *content* half is thin on Apple Music,
pushing us toward collaborative/metadata signals plus LLM-derived semantics.

**7. Spotify's public audio-features API was restricted on 27 Nov 2024 — verified.**
This is not folklore. Spotify's own developer blog ("Introducing some changes to our Web
API," dated 2024-11-27) restricts, for new/dev-mode apps without a prior extension:
**Audio Features, Audio Analysis, Recommendations, Related Artists, Get Featured Playlists,
Get Category's Playlists, 30-second preview URLs in multi-get responses, and algorithmic/
Spotify-editorial playlists** [1]. Apps with prior extended-mode access keep them [1].
Stated reason: "a more secure platform"; press coverage frames it as curbing scraping and
AI-training misuse [1][2]. Net effect: audio-feature vectors are no longer a reliable,
freely available basis for new apps — reinforcing our metadata/co-occurrence-first stance.

**8. Apple Music exposes far less than Spotify ever did.**
Apple's MusicKit/Apple Music API is built for catalog + playlist operations and returns
song/artist/album metadata; it does not publish the danceability/energy/valence-style
audio-feature vectors Spotify historically offered [15]. So even before the Spotify
deprecation, an Apple-centric tool like ours never had rich per-track audio features to
begin with — the constraint is structural, not just recent.

**9. Preference-only recommendation causes filter bubbles and over-personalization.**
Recommending only items close to known taste narrows diversity and novelty over time and
suppresses serendipitous encounters [7][8]. The literature distinguishes three
"beyond-accuracy" goals: **diversity** (variety within a list), **novelty** (difference
from what the user has seen before), and **serendipity** (relevant *and* unexpected *and*
previously unknown) [7][9]. Whether filter bubbles are severe is debated — a systematic
review finds the evidence mixed ("fact or fallacy") — but the consensus design response is
to inject structured, weighted serendipity rather than pure accuracy maximization [7][9].

**10. Explicit and implicit feedback are not interchangeable, and saves are the strongest signal.**
Explicit feedback (saves, playlist adds, likes/dislikes, follows) is weighted more heavily
than passive listening; implicit feedback (completion, skips, repeats, session length)
is noisier but abundant [13][14]. Signal semantics matter: an early skip (<5s) differs from
a late skip; completion and repeat plays are positive; a *save* is reported as the single
strongest explicit approval signal [13][14]. For us, the act of putting a song in a saved
playlist is itself a strong explicit endorsement — our whole input is high-quality signal.

## Principles we can operationalize

Given our JSON exports (title / artist / album per track; playlist names; liked songs) and
**no rich audio features**, here is what is directly computable:

- **Distributional taste profile.** Aggregate artist frequency, inferred genre (via artist
  → genre lookup), album/era distribution (release year buckets), and artist diversity.
  This is a cheap, explainable "who they are" summary [12].
- **Co-occurrence graph / matrix.** For every pair of songs (and every pair of artists)
  that share a playlist, increment a weight. This is the raw material for both
  neighbor-based CF and embeddings — and it is 100% metadata-derived [3][4][6].
- **Song/artist embeddings from our own playlists.** Run word2vec/skip-gram over the
  user's playlists (and, if available, a larger pooled playlist corpus) treating each
  playlist as a sentence. Represent a playlist or the whole library as the **centroid of
  its song vectors**; recommend nearest neighbors of that centroid [6].
- **Playlist-level themes.** Playlist *titles* are a semantic signal ("focus", "gym",
  "sunday morning") an LLM can interpret into mood/context tags without any audio data.
- **Sequencing patterns (soft).** Where order looks intentional (e.g. energy ramps by
  proxy signals like tempo tags if we can source them, or manual curation), treat position
  as a weak next-track hint — but do not over-trust it for saved playlists [10].
- **Feedback ledger.** Model the presence of a track in a saved playlist / liked list as an
  explicit positive; reserve fields to later record skips, saves, and completions as we
  gain them, weighting explicit over implicit [13][14].

What we should **not** pretend to have: reliable danceability/energy/valence vectors. Any
"audio vibe" reasoning has to come from artist/genre metadata or an LLM, not from Apple.

## Implications for the feature

- **Candidate generation = co-occurrence + embeddings, not audio matching.** Build the
  candidate pool from (a) neighbor-based CF over the shared-playlist co-occurrence matrix
  and (b) nearest neighbors of the library/playlist centroid in a word2vec-style embedding.
  Both are metadata-only and align with the strongest published signals [3][4][6][12].
- **Frame the task as APC.** "Extend this playlist" and "build from your library" are
  literally the RecSys-2018 APC task; seed with a subset of the user's tracks + the
  playlist title and rank a large candidate set [3].
- **Where an LLM helps.** (i) Turn playlist *titles* and the artist/genre/era distribution
  into human-readable taste and mood/context tags; (ii) supply *semantic* similarity where
  we lack audio features (an LLM knows "these two artists share a scene/era/mood"); (iii)
  inject controlled **novelty/serendipity** — ask for adjacent-but-unfamiliar artists to
  counter the filter bubble — with a tunable diversity knob rather than pure similarity
  maximization [7][9]. Keep the LLM as a re-ranker/explainer over CF/embedding candidates,
  not the sole retriever (it hallucinates track existence; our matcher must verify against
  the Apple catalog).
- **Deliberately budget for diversity and discovery.** Reserve a fraction of each generated
  playlist for novel/serendipitous picks and report it, so we optimize satisfaction and
  discovery, not just nearest-neighbor accuracy [7][9][12].
- **Collect feedback signals from day one.** Log which suggested tracks the user keeps vs.
  removes (explicit), and — if we ever observe playback — skips/completions/repeats
  (implicit), weighting explicit higher [13][14]. Even simple keep/remove is a save-grade
  signal we can learn from.

## Open questions

- Do we have (or can we ethically pool) a **larger playlist corpus** beyond one user's
  library? Single-user co-occurrence is sparse; embeddings and CF both improve massively
  with volume. Without pooling, is metadata + LLM enough?
- How much can an **LLM substitute for audio features** in practice? Can it reliably rank
  "vibe" similarity from title/artist/album alone, and how do we evaluate that offline?
- What is our **offline evaluation** metric for a metadata-only APC pipeline (R-precision /
  NDCG on held-out tracks from the user's own playlists)?
- How do we **tune the novelty/serendipity knob** per user without an interaction history —
  a sensible default, then adapt from keep/remove feedback?
- Can we source **any** lightweight audio proxies (tempo, key) from a non-Apple service to
  enrich sequencing, and is the added complexity worth it given the deprecation landscape?
- Where is the line between **respecting the user's grouping** (their playlists encode
  intent) and **improving on it** (deduping, re-sequencing, cross-pollinating)?

## Sources

- [1] Introducing some changes to our Web API — Spotify for Developers — https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api — Primary/authoritative: lists the exact endpoints restricted on 2024-11-27 (Audio Features, Audio Analysis, Recommendations, Related Artists, featured/category playlists, previews), who is affected, and the "more secure platform" rationale.
- [2] Spotify cuts developer access to several of its recommendation features — TechCrunch (2024-11-27) — https://techcrunch.com/2024/11/27/spotify-cuts-developer-access-to-several-of-its-recommendation-features/ — Press confirmation and framing (anti-scraping / AI-training concerns).
- [3] An Analysis of Approaches Taken in the ACM RecSys Challenge 2018 for Automatic Music Playlist Continuation — arXiv:1810.01520 — https://arxiv.org/pdf/1810.01520 — Defines the APC task on the Million Playlist Dataset, participation stats, winning scores, and that neighbor-based methods were competitive.
- [4] Automatic Playlist Continuation through a Composition of Collaborative Filters — arXiv:1808.04288 — https://arxiv.org/abs/1808.04288 — A top challenge approach: combine multiple CF views of a playlist, tuned with a Tree-structured Parzen Estimator.
- [5] A Scalable Framework for Automatic Playlist Continuation on Music Streaming Services — arXiv:2304.09061 — https://arxiv.org/pdf/2304.09061 — Production-oriented APC; corroborates neighborhood/embedding methods and scalability concerns.
- [6] Using Word2vec for Music Recommendations — Ramzi Karam (Anghami), Towards Data Science — https://medium.com/towards-data-science/using-word2vec-for-music-recommendations-bb9649ac2484 — Playlists-as-sentences skip-gram; songs that co-occur cluster; user taste as the average (centroid) of song vectors; in production at Anghami. No audio features required.
- [7] Filter Bubbles in Recommender Systems: Fact or Fallacy — A Systematic Review — arXiv:2307.01221 — https://arxiv.org/html/2307.01221 — Reviews filter-bubble evidence (mixed) and beyond-accuracy remedies: diversity, novelty, serendipity.
- [8] Counteracting the filter bubble in recommender systems: Novelty-aware matrix factorization — ResearchGate — https://www.researchgate.net/publication/335376707 — On over-personalization narrowing diversity/novelty and CF-level mitigations.
- [9] Design of a Serendipity-Incorporated Recommender System — MDPI Electronics 14(4):821 — https://www.mdpi.com/2079-9292/14/4/821 — Defines diversity/novelty/serendipity and argues for structured/weighted serendipity over pure randomness or pure accuracy.
- [10] Transformers4Rec: Bridging the Gap between NLP and Sequential/Session-Based Recommendation — ACM RecSys 2021 — https://dl.acm.org/doi/fullHtml/10.1145/3460231.3474255 — Session-based/sequence models; context for GRU4Rec → SASRec/BERT4Rec and self-attention for next-item.
- [12] Inside Spotify's Recommendation System: Complete Guide — Music Tomorrow — https://music-tomorrow.com/blog/how-spotify-recommendation-system-works-complete-guide — Hybrid architecture: content-based (audio + LLM embeddings) + collaborative "organizational similarity" (same-playlist co-occurrence over ~700M playlists) + user-taste embeddings; cold-start and diversity/discovery goals.
- [13] Decoding the Spotify Algorithm: Skip Rate, Save Rate & Playlist Adds — Artistrack — https://artistrack.com/spotify-algorithm-skip-rate-save-rate/ — Signal semantics: early vs late skips, completion, repeats; saves as strongest approval signal.
- [14] Spotify Recommendation System Explained (2026) — Chartlex — https://www.chartlex.com/blog/streaming/spotify-recommendation-system-explained-2026 — Explicit vs implicit vs contextual signals and their relative weighting (explicit action > passive listening).
- [15] Spotify API Alternatives / Best Music APIs — Zuplo — https://zuplo.com/blog/2024/12/02/spotify-api-alternatives — Notes Apple Music API is catalog/playlist-oriented and does not expose Spotify-style audio-feature vectors; context on the post-deprecation landscape. (Weaker source; Apple's own MusicKit docs would be the authoritative confirmation.)
