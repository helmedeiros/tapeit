# Contributing

Thanks for your interest. There are two kinds of contributions:

1. **Code** — improvements to the `tapeit` CLI.
2. **Playlists** — portable playlist lists under [`playlists/`](playlists/).

## Contributing a playlist

The fastest way to contribute. See [`playlists/README.md`](playlists/README.md)
for the format. In short: add a `<slug>.json` at the top level with at least
`title` and `artist` per track; include `isrc`/`ids` when you have them. Layout
is flat and attribution is the git history — no per-person folders. If a
playlist you add already exists, **merge** the two into one richer list rather
than renaming yours.

You can generate one from your own library with `tapeit import apple` /
`tapeit import spotify`, or write it by hand.

## Contributing code

### Setup

- Go **1.26+**.
- `make build` produces `./bin/tapeit`.

### Quality gates — must be green

```bash
make check      # fmt + vet + lint + test (race) — run before every commit
```

CI runs the same checks on every pull request; a red build won't be merged.

### Conventions

- **Conventional Commits**: `type(scope): subject` (e.g. `feat(import): …`).
- Small, focused commits — one logical change each, independently revertable.
- Match the surrounding code's style and comment density. The codebase favors
  clean names over explanatory comments; keep comments to the non-obvious *why*.
- Architecture and the port/adapter boundaries: [`docs/DESIGN.md`](docs/DESIGN.md).
  Decisions and trade-offs: [`docs/DECISIONS.md`](docs/DECISIONS.md).

### Pull requests

1. Fork and branch off `main`.
2. Make the change with tests; keep `make check` green.
3. Open a PR describing the change and why. Link any related issue.

## Security

Never commit credentials — see [`SECURITY.md`](SECURITY.md). Tokens live in your
OS config dir, never in the repo.

## Scope

The **code** is a personal-use tool for moving and backing up **your own**
library (it uses the Apple web-player token outside Apple's published API terms —
see the README). The **`playlists/` collection** is open for anyone to share to.
Contributions that broaden either, without crossing into other people's accounts
or sanctioned-API circumvention beyond what's already documented, are welcome.
