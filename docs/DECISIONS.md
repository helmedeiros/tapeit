# Decisions & verified facts

## Settled (this planning session)

| Decision            | Choice                                                            |
| ------------------- | ---------------------------------------------------------------- |
| Direction           | Spotify → Apple Music, one-time migration                        |
| Scope               | Owned playlists **+ followed playlists + Liked Songs**           |
| Interface           | CLI                                                              |
| Language            | Go (1.26+)                                                       |
| Apple access        | **No** paid Developer account — web-player token (unofficial)    |
| Repository          | Private, separate from dotfiles                                  |
| Architecture        | Hexagonal, SOLID, clean code, high quality gates, small commits  |

## Open questions

- **Spotify Premium**: dev-mode apps require the owner to have active Premium
  (Feb 2026). Confirm you have/will reactivate it — otherwise the whole source
  side is blocked.
- **Liked Songs target name / split**: single playlist `"Liked Songs (from
  Spotify)"`, or leave configurable? (Default: single playlist.)
- **Followed playlists**: recreate as your own library copies (only option via
  the API). Confirm that's the desired behavior vs. skipping them.
- **Device-sync verification**: do an early empirical check that a
  web-token-created playlist appears on an iPhone with Sync Library on.

## Verified facts (June 2026) — sources

### Apple write via web-player token — CONFIRMED viable
- Working OSS reference: <https://github.com/Myp3a/apple-music-api> (creates
  library playlists + adds tracks with Bearer dev token + `media-user-token`).
- Constraints + cookie name + write rules: <https://www.music-assistant.io/music-providers/apple-music/>
- Token extraction methods: <https://github.com/OrfiDev/orpheusdl-applemusic-basic/blob/main/applemusic_api.py>
- Host `amp-api.music.apple.com`, requires `Origin: https://music.apple.com`.
- Tokens expire ~180 days, non-renewable; technique is brittle / ToS gray area.
- Request shapes: Apple docs `libraryplaylistcreationrequest`,
  `add-tracks-to-a-library-playlist`.

### Spotify reads — CONFIRMED available in dev mode
- 🔴 App owner must have **Premium** (Feb 2026):
  <https://developer.spotify.com/documentation/web-api/concepts/quota-modes>
- Feb 2026 changes (5-user allowlist, `/playlists/{id}/items` rename,
  `/users/{id}/playlists` removed, `/tracks?ids=` removed from dev mode):
  <https://developer.spotify.com/documentation/web-api/references/changes/february-2026>
- `/me/playlists` returns owned **and** followed:
  <https://developer.spotify.com/documentation/web-api/reference/get-a-list-of-current-users-playlists>
- Nov 2024 deprecations don't touch personal-library reads:
  <https://developer.spotify.com/blog/2024-11-27-changes-to-the-web-api>
- PKCE + loopback, `127.0.0.1` only (no `localhost`):
  <https://developer.spotify.com/documentation/web-api/concepts/redirect_uri>
- `external_ids.isrc` intact (removal proposed Feb 2026, **reverted** Mar 2026 —
  keep monitoring + fallback matcher):
  <https://developer.spotify.com/documentation/web-api/references/changes/march-2026>

### Apple ISRC matching — CONFIRMED
- `filter[isrc]`, comma-separated, max 25 songs/response, one ISRC → many songs:
  <https://developer.apple.com/documentation/applemusicapi/get-multiple-catalog-songs-by-isrc>
- Catalog reads need dev token only (no user token).
- `/v1/me/storefront` for storefront id (needs user token; allow manual override).
- Text-search fallback carries `durationInMillis` + `isrc` for scoring.
- Expect a non-trivial unmatched rate (regional/catalog gaps, ISRC variance,
  occasional 404 on a returned id) — surface for manual review.
