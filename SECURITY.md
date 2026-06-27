# Security

## Credentials never live in this repo

`tapeit` talks to Spotify and Apple Music with **your own** credentials, and it
stores them **outside the repository** — in your OS user config directory
(`os.UserConfigDir()/tapeit/`, e.g. `~/Library/Application Support/tapeit/` on
macOS):

- `spotify_token.json` — Spotify OAuth token (PKCE; no client secret stored).
- `apple_creds.json` — the Apple Music web-player **developer token** and your
  **`media-user-token`** cookie.

Treat both like passwords. The Apple tokens in particular grant access to your
library; anyone holding them can read and modify it until they expire (~180
days).

**Never commit credentials.** `.gitignore` excludes build output and local
state, and no token is read from or written to the repo. If you ever paste a
token into a file under the repo, do not commit it — and if one is committed,
rotate it (re-extract the Apple tokens / re-auth Spotify) rather than only
deleting the file, since git history retains it.

The data under `playlists/` is intentionally shareable: track titles, artists,
albums, durations, public catalog ids, and ISRCs. It contains **no** account
ids, tokens, or other secrets.

## Reporting a vulnerability

Please report security issues privately via GitHub's **"Report a vulnerability"**
flow (Security → Advisories) rather than opening a public issue. We'll
acknowledge and respond as soon as we reasonably can.

## Scope note

`tapeit` uses the Apple Music web player's token outside Apple's published API
terms, against your own library only (see the README). This is a personal-use
trade-off, not a sanctioned integration; it is unrelated to the credential
handling above.
