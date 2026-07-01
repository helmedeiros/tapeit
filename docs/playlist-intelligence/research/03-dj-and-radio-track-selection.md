# DJ and radio track selection

*Part of the Playlist Intelligence corpus — research on how professional selectors decide what to play next, and what of it we can borrow for automated sequencing.*

## Why this matters for us

The hardest part of building a listener's personal playlist is not *which songs are good* — it is *what order they go in* and *what comes next given what just played*. Two communities have spent decades formalizing exactly this decision under real pressure: club DJs (deciding the next track live while watching a crowd) and radio programmers (deciding the next track offline, but at scale, to hold an audience across hours). Between them they have produced both an intuitive vocabulary (energy, journey, tension/release, "reading the room") and a hard, codified rule system (rotation categories, artist/title separation, hot clocks, dayparting, mood/energy scoring) baked into commercial scheduling software. Our auto-sequencing and curation logic can lift directly from the codified layer — separation and energy-flow rules are literally already numeric — while treating the intuitive layer as the design goal the numbers are trying to approximate.

## Key findings

### Club DJs — reading a live crowd

**It's observation, not guessing.** Crowd-reading starts before the first record — DJs read the room's energy as people arrive and track *changing movement patterns* and *distribution* on the floor as continuous feedback: whether people dance exuberantly or reservedly, whether the front (near the booth) is full, and whether guests face the booth or the exits [1][2]. Two signals recur as opposite poles: a full dancefloor at the front = high energy; phones and conversation coming out = the set is losing tension [1][3].

**The set is a shaped arc, not a queue.** Practitioners describe "energy flow" as *the shape of a set over time — when to build tension, when to release it, when to peak, and when to let the floor breathe* [4][5]. The craft is building energy gradually, peaking at the right moment, then deliberately giving the audience room to recover before lifting them again — extended builds, a release, then space [1][4]. The metaphor of "taking them on a journey" / a "dramatic arc" is explicit and near-universal across the DJ-education sources [1][6].

**Plan vs. improvise — hold a rough structure, react in real time.** The consistent advice is to prepare *a rough structure* but treat the live set *like a conversation rather than a performance* — offer a track, watch the response, then decide what comes next [1][6]. The ability to *abandon a planned direction in favor of what the room needs* is repeatedly named as what separates good sets from unforgettable ones [1][6]. Tools (hot cues, loops, performance pads, waveform displays, rekordbox energy/BPM markings) exist specifically to keep that flexibility without losing the arc [6].

**Don't over-react to every dip.** A named failure mode is "hectic changes as soon as the energy weakens a little" — DJs are told to distinguish genuine disinterest (phones appearing) from a natural breathing moment (head-nodding), and read the *overall mood* rather than twitch on every dip [6].

**Phrasing / timing / trainwreck avoidance.** Beyond song choice, mechanical fit matters: mixing on phrase boundaries and matching BPM/structure so transitions land cleanly; a "trainwreck" is the beat-mismatch that breaks the floor's trance. Waveform and BPM displays are used precisely to time drops and avoid this [6]. (This is the club analogue of radio's "separation" and "flow" — the mix has to feel inevitable.)

### Radio programming — deciding at scale, to a format

**The hot clock / format clock is the skeleton.** A format (or "hot") clock is an hour-template specifying *which song category plays at each position* within the hour, with slots for talk breaks, ads, and IDs [7][8]. Stations build several clock variants and rotate them across dayparts and days so heavy listeners perceive freshness while the format stays consistent [8][9]. A concrete rotation tip: hold ~7 songs (not 6) in Power Current because *it takes 7 hours before all songs fall into the same slots*, which produces an uneven, non-predictable rotation across dayparts and days [10].

**Rotation categories describe a song's life cycle.** Songs are tiered by popularity/familiarity, and the tier sets the play frequency [9][11]:
- **Power / Power Currents** — the most popular current hits, played many times a day (heaviest rotation).
- **Secondary Currents** — newer songs still building familiarity (freshness) or former powers on the way down.
- **Recurrents / Power Recurrents** — recent former hits the audience can already sing along to; used to inject familiarity without burning currents [12].
- **Gold** — older catalog, minimal rotation, often split into era sub-categories for era balance [9][11].

**Rotation math is deliberate.** Turnover (how fast a category refreshes) is tuned by the ratio of clock slots to songs in the category — e.g. *48 slots / 5 songs = 9.6 plays per song per day* — and programmers intentionally aim for an *uneven* (non-integer) ratio so songs don't lock into the same hours every day [8]. Category size is adjusted week to week against the available song pool [8].

**Dayparting.** The broadcast day is divided into segments (morning drive, midday, afternoon drive, evening, overnight, weekends), and stations run daypart-specific clocks — e.g. morning shows use clocks with more talk breaks and fewer songs than middays, and afternoons/evenings/weekends get their own grids [13][8]. This tailors category emphasis, talk load, and mood to the audience and mode of listening at that hour.

**Separation rules keep it from feeling repetitive.** Scheduling enforces minimum gaps: **Title separation** (minimum interval before the same song repeats), **Artist separation** (minimum interval before the same artist), and **characteristic separation** (how many songs sharing an attribute — tempo, sound code, mood — can play in a row) [14]. Sound-code rules ("don't play two hard-rock songs back to back") and daypart restrictions ("don't play this song in mornings") are classic examples [15].

**Tempo and mood are separate, scored axes.** In MusicMaster, every song is scored on **Mood** (1 = very sad → 5 = very happy), **Energy** (1 = very low → 5 = very high), and **Tempo** (1 = very slow → 5 = very fast, or BPM) [14]. Keeping tempo and mood distinct lets the scheduler do things like run a *slow-tempo / exciting-mood* song into an up-tempo track to pick up the pace, or use mood rules to avoid stacking back-to-back sad songs [14].

**Callout & request research decide what's hot and what's burned.** Music directors don't rely on gut alone. **Callout research** plays song *hooks* down a phone line to the target audience (weekly/biweekly, ~30–40 titles per wave — respondents lose concentration beyond ~30) and rates each on a scale like *Favourite / Like / Neutral / Burn / Negative / Unfamiliar* [16][17]. This tracks each song's life cycle from exposure to peak to decline via **"burn"** — the point where listeners tire of a song — and tighter rotations burn songs faster [16][17]. **Auditorium music testing** tests far larger libraries (gold) in a single session [16]. Research is focused on the station's **P1** (most loyal) listeners, since they drive ratings [16][18].

### Scheduling software — what it actually optimizes

**Selector (rule-based, 1979).** The original RCS Selector, by Dr. Andrew Economos, decided each position by *rules that forbid things*: no two rock songs in a row, no Beatles song within 2 hours, this song never in mornings, etc. [19].

**GSelector (goal-based, 2006).** RCS reinvented this as *goal-driven, demand-based* scheduling (US-patented): instead of only forbidding, the music director sets **goals/demand** for attributes — tempo, energy, mood, artist occurrence — and the scheduler adjusts to hit those desired outcomes [19]. GSelector's goal-driven scheduler evaluates every candidate song against station priorities — artist/title separation, tempo/energy flow, sound codes, daypart rules, custom attributes — to build a balanced log [20]. Notably, its Artist field is informational; separation is driven by "participants" (per-song contributors), and a **participation %** setting lets a background vocalist count less, so their other songs can schedule closer [20]. Analysis tooling audits actual spin counts, category turnover, and vocalist minimum-separation to confirm the library rotates as intended [21].

**MusicMaster.** Uses per-song attributes (tempo, mood, energy) plus separation rules (title/artist/characteristic) to control *flow, balance, and mix* of the scheduled log [14][22].

The common thread: both systems reduce "what plays next" to (1) hard separation constraints + (2) a soft optimization over energy/mood/tempo/attribute *flow* toward target proportions.

### The human judgment layer

Across both worlds, experienced selectors describe looking for the same things in the next track: *does it move energy in the direction the room/hour needs* (build, hold, release, or peak); *is it fresh enough not to feel repetitive but familiar enough to reward* (the currents-vs-recurrents-vs-gold balance; the burn/familiarity trade-off); *does it transition cleanly* (phrasing/BPM for DJs, mood/tempo flow for radio); and *does it respect the arc* — nobody just picks "the next good song," they pick the next song *given where we are in the journey* [1][4][14][17].

## Principles we can operationalize

1. **Score every track on separate energy, mood, and tempo axes** (radio's 1–5 model), not one blended "vibe" number — so we can raise energy while lowering mood, or vice-versa [14].
2. **Enforce separation as hard constraints.** Minimum gaps for same-artist, same-album/title, and same-characteristic (genre/tempo/era) runs — the single most portable radio rule [14][15].
3. **Shape an energy arc, not a flat shuffle.** Model a target energy curve over the playlist (intro → build → peak → breathe → lift), and choose each next track to move toward the curve's next point — the club "journey" as a numeric target [4][5][14].
4. **Adopt rotation tiers for repeat listening.** For playlists the user replays, borrow currents/recurrents/gold: favorites (power) recur more, but with separation and an uneven cadence so they don't land in the same spot every listen [9][10].
5. **Aim for uneven, non-integer cadence** so repeats feel fresh — the "7 not 6 / 9.6 plays" insight [8][10].
6. **Model "burn."** Track per-user familiarity/fatigue and demote overplayed tracks over time, the way callout research demotes burned songs — tighter rotation should burn faster [16][17].
7. **Daypart / context clocks.** Vary the target energy/mood curve by context (morning vs. workout vs. wind-down) the way radio varies clocks by daypart [13][8].
8. **Goal-based over rule-based, where possible.** Follow GSelector: express desired *proportions* (X% high-energy, artist demand) and optimize toward them, rather than only listing forbidden adjacencies [19][20].
9. **Clean transitions matter.** Where we have BPM/key/tempo data, prefer next tracks that segue cleanly (radio's flow / DJ's phrasing) — avoid the "trainwreck" jump [6][14].

## Implications for the feature

- Our sequencer should be two layers: a **hard-constraint pass** (separation) and a **soft optimization pass** (energy/mood/tempo flow toward a target arc) — mirroring how both GSelector and MusicMaster are architected [14][19][20].
- A single "energy" tag is insufficient. We need at least energy + mood + tempo per track (derivable from audio features / BPM) to reproduce radio-quality flow [14].
- For replayed/living playlists we need a **rotation model with per-user burn tracking**, not just a static order [16][17].
- "Take them on a journey" is implementable as a **target energy curve** the picker steers toward — this is probably our highest-leverage borrowing, because it converts a vague aesthetic into an objective function [4][5].
- We should expose **context presets** (daypart clocks) rather than one universal ordering [13][8].

## Open questions

- What energy/mood/tempo signals can we actually get per track (Apple Music / audio-feature availability) to populate the radio-style scoring axes? Sources describe the *model*, not where to source the numbers for our catalog.
- How aggressively should personal-playlist separation differ from radio? Radio serves a broad audience; a solo listener may *want* their favorites closer together. Unverified — needs user testing.
- Is a target-energy-curve worth the complexity for short (~20-track) personal playlists, or only for long/continuous listening? The DJ/radio sources assume multi-hour sessions.
- How to detect and model per-user "burn" without explicit callout-style feedback — implicit signals (skips, replays) as a proxy is plausible but unproven here.
- Transition quality (BPM/key harmonic mixing) is well-documented for DJs but I did not find quantified rules we can lift directly; would need dedicated research into harmonic-mixing / Camelot-wheel practice.

## Sources

[1] Relentless Beats — *Behind the Booth: How DJs Read a Crowd and Control a Night's Energy* — https://relentlessbeats.com/2026/02/behind-the-booth-how-djs-read-a-crowd-and-control-a-nights-energy/ — DJ-blog piece on crowd cues, energy control, tension/release, plan-vs-improvise.
[2] Bauhaus Las Vegas — *DJ Spotlight: The Art of Reading a Crowd and Building Energy on the Dance Floor* — https://bauhauslv.com/blogs/dj-spotlight-the-art-of-reading-a-crowd-and-building-energy-on-the-dance-floor/ — venue blog on reading floor distribution/movement.
[3] Point Blank Music School — *How to Create Engaging DJ Sets That Keep the Crowd Dancing* — https://www.pointblankmusicschool.com/blog/how-to-create-engaging-dj-sets-that-keep-the-crowd-dancing/ — DJ-education guide on set engagement and energy cues.
[4] SetFlow — *DJ Set Energy Flow: How to Structure Sets* — https://www.setflow.app/blog/dj-set-energy-flow — defines "energy flow" as the shape of a set (build/release/peak/breathe).
[5] Mixgraph — *Understanding Energy Flow in DJ Sets* — https://www.mixgraph.io/learn/energy-flow-guide — energy-arc structuring guide.
[6] Recordcase — *Crowdreading for DJs — Read the Room, Master the Set (2025)* — https://www.recordcase.de/en/crowdreading-dj-guide-2025 — detailed crowd-reading cues, plan-vs-improvise, over-reaction failure mode, tools (hot cues, waveforms, rekordbox).
[7] Radio ILOVEIT — *Top 40 / CHR Music Scheduling Format Clocks (1)* — https://radioiloveit.com/radio-music-research-music-scheduling-software/top-40-radio-format-chr-contemporary-hit-radio-music-scheduling-format-clocks-1/ — format-clock explainer for CHR.
[8] Radio ILOVEIT — *Music Scheduling: Using Song Rotations for Better Music Logs* — https://radioiloveit.com/radio-music-research-music-scheduling-software/music-scheduling-using-song-rotations-for-better-music-logs/ — rotation math (slots÷songs), turnover, uneven cadence, clock variants, daypart clocks.
[9] Radio ILOVEIT — *Song Categories and Music Formats / Format Clocks* — https://radioiloveit.com/radio-music-research-music-scheduling-software/radio-music-scheduling-tips-and-music-radio-programming-advice-on-song-categories-and-music-formats-and-format-clocks/ — currents/recurrents/gold category definitions and era balance.
[10] Powergold — *12 CHR Music Format Clocks You Can Adjust & Apply Today* — https://powergold.com/12-chr-music-format-clocks-you-can-adjust-apply-today-download/ — the "7 not 6 Power Current" uneven-rotation guidance.
[11] Powergold — *Music Scheduling Q&A #1 — How to Build Your Song Categories* — https://powergold.com/music-scheduling-qa-1-how-to-build-your-song-categories/ — category construction (currents/recurrents/gold, subcategories).
[12] Radio ILOVEIT — *How Power Recurrents Improve Radio Ratings* — https://radioiloveit.com/radio-music-research-music-scheduling-software/music-scheduling-how-power-recurrents-improve-radio-ratings/ — role of power recurrents (familiar recent hits).
[13] Fiveable — *Dayparting (Radio Station Management)* — https://fiveable.me/radio-station-management/unit-2/dayparting/study-guide/3XgOJyh4EQMCEIkq — dayparting concept: dividing the day, variety/repetition control, demographic tailoring (accessed via search snippet; direct fetch returned 404).
[14] MusicMaster / Wikipedia + vendor docs — *MusicMaster (software)* — https://en.wikipedia.org/wiki/MusicMaster_(software) and https://musicmaster.com/?tag=rules — Mood/Energy/Tempo 1–5 scoring, tempo-vs-mood distinction, title/artist/characteristic separation, flow/balance/mix rules.
[15] RCS Sound Software — *Make the Switch to GSelector* — https://www.rcsworks.com/make-the-switch-to-gselector/ — examples of Selector rules (sound-code, artist separation, daypart restriction).
[16] Radio ILOVEIT — *Radio Research: Auditorium Music Testing & Callout Research* — https://radioiloveit.com/radio-music-research-music-scheduling-software/radio-research-auditorium-music-testing-and-callout-research/ — callout mechanics, hooks, life-cycle/burn, auditorium testing, P1 focus.
[17] Radio ILOVEIT — *Callout Music Research Tips to Test Currents & Recurrents (Part 1)* — https://radioiloveit.com/radio-music-research-music-scheduling-software/callout-radio-music-research-tips-to-test-currents-and-recurrents-part-1/ — 6-point scale (Favourite/Like/Neutral/Burn/Negative/Unfamiliar), ~30 titles per session, burn vs rotation tightness.
[18] Coleman Insights — *Why Does My Radio Station Need Callout Research?* — https://colemaninsights.com/coleman-insights-blog/why-does-my-radio-station-need-callout-research — purpose of callout, P1-audience focus.
[19] RCS via search + Wikipedia — *Radio Computing Services / GSelector goal-based scheduling* — https://en.wikipedia.org/wiki/Radio_Computing_Services and https://www.rcsworks.com/make-the-switch-to-gselector/ — Selector (1979, rule-based, Economos) vs GSelector (2006, goal/demand-based, patented).
[20] RCS Sound Software — *Artist Control — Then and Now* (and GSelector product page) — https://www.rcsworks.com/artist-control-then-and-now/ and https://www.rcsworks.com/gselector/ — participants vs Artist field, participation-% separation, goal-driven evaluation across separation/flow/daypart/attributes.
[21] RCS Sound Software — *Understanding GSelector's Analysis Feature* — https://www.rcsworks.com/understanding-gselectors-analysis-feature/ — spin-count/turnover/vocalist-separation auditing.
[22] MusicMaster — official site (rules tag) — https://musicmaster.com/?tag=rules — vendor description of attribute-driven flow and scheduling rules.
