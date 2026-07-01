# Measuring playlist success

*Part of the Playlist Intelligence research corpus.*

## Why this matters for us

We generate playlists (e.g. `tapeit create` building an Apple Music playlist from a
supplied song list) and we need a defensible answer to the question **"is this
playlist any good?"** — ideally before a human ever presses play.

The hard constraint: **we currently have no playback telemetry.** We do not see
completion rates, skips, saves, or follower growth. The vast majority of the
music-industry literature on playlist "success" is built on exactly those signals,
so it describes an evaluation regime we cannot yet run. That pushes us toward the
three families of evaluation that *don't* need our own live listeners:

1. **Offline evaluation** — treat playlist generation as a recommendation/ranking
   task and score it against held-out ground truth (e.g. hide tracks from a real
   playlist and see if we recover them).
2. **Human / LLM rubric evaluation** — judge coherence, flow, and fit-to-intent
   directly from the tracklist and metadata.
3. **Proxy / content signals** — diversity, novelty, artist balance, tempo/mood
   coherence computed from track features, plus match/coverage quality of our own
   pipeline.

Live engagement and retention metrics are documented here too, because they are the
**gold standard we are proxying for** and the thing we would graduate to if/when we
ever collect telemetry.

## Key findings

### Engagement metrics (the industry gold standard — requires telemetry we lack)

Streaming platforms and the analytics ecosystem around them converge on a small set
of per-track / per-playlist engagement signals:

- **Completion / finish rate** is treated as the single strongest per-play
  satisfaction signal; the first ~30 seconds are the critical window, and tracks
  skipped inside that window are penalized in algorithmic distribution [1][2][6].
- **Skip rate**, especially **early skips within the first 30 seconds**, is a
  negative-quality signal that suppresses radio/autoplay/algorithmic placement
  [1][2][6].
- **Save / add rate** (saves ÷ unique listeners) is repeatedly described as the
  highest-confidence positive signal — "I want this again." Industry writing cites
  **~3–5% as a target band** and claims saves now outweigh raw stream volume for
  algorithmic ranking, though these specific numbers are vendor/marketing claims and
  should be treated as directional, not authoritative [1][3][4].
- **Stream-to-listener ratio** (replays per unique listener), with a cited "healthy"
  band of ~1.5–2.0, is used as a proxy for genuine engagement vs. passive play —
  again a vendor benchmark [1].
- Additional engagement signals commonly listed: **likes/hearts, share rate,
  session length, tracks-per-session, and playlist adds** [2][3][4].
- Note: these thresholds come from artist-marketing/analytics vendors (Chartlex,
  Artistrack, Boost Collective), not from peer-reviewed work or official Spotify
  documentation. The *direction* (completion↑ good, early-skip↑ bad, save↑ good) is
  consistent across sources; the *exact* percentages are uncertain [1][2][3][4].

### Retention / longer-term

- Beyond per-play signals, the longer-horizon metrics cited are **listener return
  rate** and **playlist follower growth** — i.e. do people come back, and does the
  playlist accumulate followers over time [3][5]. These are the metrics most
  resistant to gaming but require sustained telemetry we do not have.

### Offline evaluation (directly applicable to us)

The recommender-systems framing: treat "continue / build this playlist" as
**automatic playlist continuation (APC)** — a sequential/top-N recommendation task —
and score predictions against a held-out set [7][8].

- **The reference benchmark is the RecSys Challenge 2018 / Spotify Million Playlist
  Dataset (MPD)**: 1,000,000 real user playlists, >2M unique tracks, ~300k artists.
  The task: given a playlist title and *K* seed tracks (K ∈ {0, 1, 5, 10, 25, 100}),
  output 500 ranked candidate tracks [7][9].
- **Three official metrics** [9]:
  - **R-precision** — retrieved relevant tracks ÷ known relevant (held-out) tracks;
    rewards *set overlap* regardless of order.
  - **NDCG** — normalized discounted cumulative gain; rewards putting relevant
    tracks *higher* in the ranked list (order-sensitive).
  - **Recommended Songs Clicks** — how many 10-track "refreshes" before the first
    relevant track appears (lower is better; 51 if none found), simulating Spotify's
    real UI.
  - Final ranking aggregated across all three via Borda count.
- **Benchmark scores**: the top main-track team reached **R-precision ≈ 0.2241,
  NDCG ≈ 0.3946, clicks ≈ 1.784**; creative track was near-identical. This tells us
  what "state of the art" recovery looks like — even the best systems recover only
  ~22% of held-out tracks, so we should not expect near-1.0 scores [7][8].
- General offline top-N metrics we can adopt directly: **Hit-Rate@k, Recall@k,
  Precision@k, MAP, NDCG@k** [10][14]. A common protocol is **leave-one-out** (hide
  the last / a random track, check if the model recovers it) with a **chronological
  / temporal split** to avoid leakage from the future [10][14].
- **Known limitation**: offline metrics are "a compass, not a map." They suffer from
  data-delivery bias (history reflects the *old* system, not true preference),
  cold-start bias, and observational bias (behavior changes once a new system ships).
  Strong offline numbers do **not** guarantee real-world success [10].

### Online evaluation (aspirational for us — needs live traffic)

- The industry-standard method is **A/B testing** of recommender/playlist variants
  on real users, measuring interventional impact rather than fit to history [10][11].
- Spotify's own experimentation framing separates metrics into **success metrics**
  (what the change should improve), **guardrail metrics** (must not regress beyond a
  threshold — evaluated with non-inferiority tests), and **deterioration metrics**
  [12][13]. Guardrails are the mechanism for catching harmful trade-offs (e.g. a
  change that boosts clicks but tanks long-term retention) [12][13].
- Related online techniques: **interleaving** and **multi-armed bandits** for faster
  comparative testing [10]. Choosing sensitive, low-variance metrics matters for
  statistical power [11].
- We cannot run this today (no users on generated playlists), but it defines the
  eventual validation loop and argues for designing guardrails now.

### Human & LLM evaluation (immediately applicable, no telemetry)

- **Rubric-based judging** — human or LLM — scores an artifact against explicit
  criteria with defined score levels ("what a 5 vs a 3 on coherence looks like")
  [15][16][17]. Recommended practice: **analytic rubrics with 3–7 criteria**, each
  tied to a failure mode you actually care about, using ordinal scales for graded
  qualities like coherence [16][17].
- **LLM-as-a-judge** uses one model to score another's output; it performs
  comparatively well on *structural* criteria such as **coherence and completeness**,
  which maps cleanly onto playlist qualities: **musical coherence, flow/transitions
  between tracks, thematic consistency, and fit-to-intent** [15][16][17].
- Research shows LLMs can even *generate* task-specific rubric dimensions and scales
  [16]. But the well-documented **cons** apply: position/verbosity/self-preference
  biases, need for validation against human labels, and instability without a fixed
  rubric [15][17]. Treat LLM scores as a cheap, scalable *screen*, calibrated against
  a small human-judged sample — not ground truth.

### Cold-start — measuring quality with zero playback data (our situation)

- Our position is a **global / new-item cold start**: no interaction history to learn
  from or evaluate against [18][19]. The literature's standard response is to lean on
  **content-based signals** (track/artist attributes, audio features, metadata,
  embeddings) instead of collaborative signals [18][19][20].
- Because accuracy-style metrics are unreliable when there's no interaction ground
  truth, the field advocates **"beyond-accuracy" metrics**: **diversity, novelty, and
  serendipity** [20][21]. Concretely, **intra-list diversity@K** (mean pairwise
  cosine distance between recommended items' feature vectors) is a directly
  computable quality proxy that needs no telemetry [20][21].
- This is the crux for us: with no playback data, we substitute **(a) offline
  recovery** against borrowed ground truth (public playlists / MPD-style held-out
  tracks), **(b) content-based coherence & diversity metrics**, and **(c) human/LLM
  rubric scores** — accepting each is a proxy, not proof, of real listener success.

## Principles we can operationalize

Given "no telemetry" constraints, a concrete starter metric set for a generated
playlist:

**Tier 1 — Pipeline / correctness quality (we can compute today, deterministically)**
- **Match rate**: % of requested tracks successfully matched in Apple Music
  (mismatch/near-miss is an obvious quality failure). This is our own equivalent of a
  "did we build what was asked" score.
- **Duplicate / dead-track rate**: unintended repeats, wrong-version matches.

**Tier 2 — Content coherence & diversity (compute from track/audio features)**
- **Intra-list diversity@K** (mean pairwise distance across audio/genre features) —
  guard against both monotony and incoherent scatter; aim for a *target band*, not a
  maximum [20][21].
- **Artist / album concentration** (e.g. share of the single most-frequent artist) —
  a balance guardrail.
- **Mood / tempo / key flow** — measure smoothness of transitions across the ordered
  list (a flow proxy) when audio features are available.
- **Novelty** relative to the seed/intent when discovery is the goal [20].

**Tier 3 — Offline recovery (when we have reference playlists)**
- Borrow ground truth from public/editorial or the user's own playlists: hide part of
  a real playlist, ask our system to continue it, score with **R-precision, NDCG@k,
  Recall@k, Hit-Rate@k** and MPD's **Recommended-Songs-Clicks** [9][10][14].
- Calibrate expectations to the MPD ceiling (~0.22 R-precision at SOTA) — relative
  improvement, not absolute score, is the signal [7][8].

**Tier 4 — LLM/human rubric (cheap, scalable, coherence-focused)**
- A **3–7 criterion analytic rubric** scored by an LLM judge and periodically
  validated against human raters: **coherence, flow between tracks, fit-to-intent
  (title/prompt), diversity-vs-repetition balance, and surprising-but-fitting picks
  (serendipity)** [15][16][17].
- Keep the rubric fixed and version it; report per-criterion scores, not just an
  aggregate.

**Tier 5 (future) — Online**
- If/when generated playlists reach real listeners, define **success metrics**
  (save/add rate, completion rate) and **guardrail metrics** (early-skip rate,
  session length) up front, and validate via A/B testing [11][12][13].

## Implications for the feature

- We can ship a **quality score today** without any listeners, built from Tier 1–2
  (deterministic content metrics) plus a Tier 4 LLM rubric. That is honest as long as
  we label it a *proxy* for listener satisfaction, not a measurement of it.
- **Offline recovery (Tier 3) is our strongest objective signal** but needs reference
  playlists; sourcing them (user's own libraries, public playlists, or MPD for
  R&D) is a concrete next step.
- **Design guardrails now** even though we can't test them yet — pick the
  success/guardrail split so that when telemetry arrives we're not starting from
  zero [12][13].
- **Don't over-trust any single number.** Offline metrics are biased compasses [10];
  LLM judges have known biases [15][17]; vendor engagement benchmarks are directional
  at best [1][2][3][4]. Report a small panel of complementary metrics.

## Open questions

- What reference ground truth can we ethically/practically use for offline recovery —
  the user's existing playlists, editorial playlists, or MPD-style corpora?
- What is a *good* intra-list diversity target band for our use cases? "This Is
  <Artist>" (deliberately narrow) vs. a mood mix (deliberately broad) need different
  targets — one global threshold won't fit.
- Can we get audio features (tempo, key, energy, valence) for matched Apple Music
  tracks to compute flow/coherence, or are we limited to genre/artist metadata?
- How well does an LLM rubric score correlate with human judgments of *our* playlists
  specifically? Needs a small labeled validation set before we trust it.
- Do we ever get *any* proxy telemetry (e.g. whether the user keeps vs. deletes the
  generated playlist, or edits it heavily)? Even coarse post-hoc signals would let us
  start closing the loop.

## Sources

[1] Chartlex — "Why Spotify Saves Beat Streams in 2026 (4x More Placements)" — https://www.chartlex.com/blog/streaming/spotify-algorithm-2026-retention-revolution — vendor analysis of save rate, completion, early-skip, stream-to-listener benchmarks (marketing source; treat numbers as directional).

[2] Artistrack — "Decoding the Spotify Algorithm: Skip Rate, Save Rate, & Playlist Adds" — https://artistrack.com/spotify-algorithm-skip-rate-save-rate/ — how skip/completion/save signals are described to feed algorithmic distribution.

[3] Chartlex — "Track Spotify Growth: 5 Metrics That Matter in 2026" — https://www.chartlex.com/blog/streaming/how-to-track-spotify-growth-metrics-2026 — engagement + retention metric overview.

[4] Boost Collective — "Spotify Save Rate vs Skip Rate (Which Metric Matters More)" — https://www.boost-collective.com/blog/spotify-save-rate-vs-skip-rate — comparison of save vs skip as quality signals.

[5] MusicPulse — "What Your Spotify Listener Retention Data Is Telling You" — https://www.musicpulse.app/blog/what-your-spotify-listener-retention-data-is-telling-you-and-how-to-fix-it — listener return rate / retention framing.

[6] Orphiq — "Skip Rate and Completion Rate: What They Mean" — https://orphiq.com/resources/skip-rate-completion-rate — definitions of skip vs completion and the 30-second window.

[7] Spotify Research — "RecSys Challenge 2018: Automatic Music Playlist Continuation" — https://research.atspotify.com/publications/recsys-challenge-2018-automatic-music-playlist-continuation — official task, MPD dataset (1M playlists), top scores (R-precision 0.2241, NDCG 0.3946, clicks 1.784).

[8] Zamani et al. — "An Analysis of Approaches Taken in the ACM RecSys Challenge 2018 for Automatic Music Playlist Continuation" (arXiv:1810.01520) — https://arxiv.org/abs/1810.01520 — detailed analysis of methods and the three evaluation metrics.

[9] AIcrowd — "Spotify Million Playlist Dataset Challenge" — https://www.aicrowd.com/challenges/spotify-million-playlist-dataset-challenge — precise definitions of R-precision, NDCG, and Recommended-Songs-Clicks; task spec (K seed tracks → 500 ranked candidates; Borda aggregation).

[10] Shaped.ai — "Recommender Model Evaluation: Offline vs. Online" — https://www.shaped.ai/blog/evaluating-recommender-models-offline-vs-online-evaluation — offline metrics (precision/recall/MAP/NDCG/diversity), biases of offline eval, why online A/B testing is the gold standard.

[11] "Powerful A/B-Testing Metrics and Where to Find Them" (arXiv:2407.20665) — https://arxiv.org/html/2407.20665 — choosing sensitive metrics for statistically powerful online experiments.

[12] codecompass00 — "Confident Product Decisions with Data: Inside Spotify's Risk-Aware A/B Testing Framework" — https://codecompass00.substack.com/p/spotify-product-decisions-a-b-testing-framework — success / guardrail / deterioration metric taxonomy and non-inferiority guardrail tests.

[13] Spotify Confidence — "A/B Test Bandwidth: The Currency of Innovation" — https://confidence.spotify.com/blog/ab-testing-bandwidth — Spotify's experimentation practice and metric discipline.

[14] GeeksforGeeks — "Offline Evaluation Metrics in Information Retrieval" — https://www.geeksforgeeks.org/machine-learning/offline-evaluation-metrics-in-information-retrieval/ — definitions of Hit-Rate@k, Recall@k, NDCG, and leave-one-out / hold-out protocol.

[15] Towards Data Science — "LLM-as-a-Judge: A Practical Guide" — https://towardsdatascience.com/llm-as-a-judge-a-practical-guide/ — using an LLM to score outputs, its strengths on structural criteria and its known biases.

[16] "Learning to Judge: LLMs Designing and Applying Evaluation Rubrics" (arXiv:2602.08672) — https://arxiv.org/html/2602.08672v1 — LLMs generating and applying rubric dimensions (coherence, fluency, informativeness).

[17] Adnan Masood — "Rubric-Based Evaluations & LLM-as-a-Judge: Methodologies, Biases, and Empirical Validation" — https://medium.com/@adnanmasood/rubric-based-evals-llm-as-a-judge-methodologies-and-empirical-validation-in-domain-context-71936b989e80 — analytic rubric design (3–7 criteria, ordinal scales) and validation guidance.

[18] Milvus — "How do recommender systems handle cold-start problems?" — https://milvus.io/ai-quick-reference/how-do-recommender-systems-handle-coldstart-problems — cold-start types (new user / new item / global) and content-based mitigation.

[19] Things Solver — "How to solve the cold start problem in recommender systems" — https://thingsolver.com/blog/the-cold-start-problem/ — cold-start definition and content-based/hybrid approaches.

[20] Springer (Int. J. Data Science and Analytics) — "Improving recommendation diversity and serendipity with an ontology-based algorithm for cold start environments" — https://link.springer.com/article/10.1007/s41060-023-00418-4 — beyond-accuracy metrics (diversity, serendipity) and intra-list diversity@K for cold-start.

[21] arXiv:2403.18667 — "Knowledge Graph-Based Semantic Contrastive Learning for Diversity and Cold-Start Users" — https://arxiv.org/pdf/2403.18667 — diversity/serendipity evaluation and intra-list diversity under cold start.
