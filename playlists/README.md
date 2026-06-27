# playlists

Portable, **service-agnostic** playlist files — one JSON per playlist. Each file
is a plain track list that any music service can match against, enriched with
whatever catalog metadata we've gathered as the playlist moved between services.

These double as a backup of a real library and as a small shared collection:
anyone can read them, reuse them, or contribute their own.

## File format

One file per playlist, named after a slug of its name (`This Is The Black Keys`
→ `this-is-the-black-keys.json`):

```json
{
  "name": "This Is The Black Keys",
  "tracks": [
    {
      "title": "Lonely Boy",
      "artist": "The Black Keys",
      "album": "El Camino",
      "durationMs": 193173,
      "isrc": "USABC1234567",
      "ids": { "appleMusic": "1052966684", "spotify": "0Vqar9rqGHO0xQiNV9XYWQ" }
    }
  ]
}
```

| Field        | Required | Meaning                                                              |
| ------------ | -------- | ------------------------------------------------------------------- |
| `title`      | yes      | Track title.                                                        |
| `artist`     | yes      | Artist(s); comma-separated for multiple.                            |
| `album`      | no       | Album / collection name.                                            |
| `durationMs` | no       | Track length in milliseconds. Sharpens matching.                    |
| `isrc`       | no       | International Standard Recording Code — the universal match key.    |
| `ids`        | no       | Per-service catalog ids, namespaced (`appleMusic`, `spotify`, …).   |

`title` + `artist` are all that's needed to build the playlist on a service; the
rest is enrichment. A list belongs to no single service — it only **accrues**
metadata. The first time a service recognises a track it fills in that service's
id (and an ISRC, when the service exposes one); later services match on the ISRC
when present, otherwise on the base title, and add their own id alongside.

## How these are produced

From the [`tapeit`](../README.md) CLI:

```bash
tapeit import apple        # read your Apple Music library into these files
tapeit import spotify      # fold your Spotify library in, enriching the same files
tapeit create --from playlists/<slug>.json   # build the playlist on a service
```

Import **merges** into an existing file rather than overwriting it: matched
tracks gain new metadata, the service's extra tracks are appended, and tracks the
service lacks are kept.

## Contributing

- Add a playlist as `<slug>.json` at the top level — a slug of its name,
  lower-case, non-alphanumeric runs collapsed to `-`.
- Layout is **flat**. No per-person folders: who added what lives in the git
  history, not in filenames.
- **Collisions merge, they don't clash.** If a playlist you add already exists,
  combine the two into one richer list (same idea as service merging — match by
  ISRC, else base title) rather than renaming yours.
- Only `title` and `artist` are required; include `isrc`/`ids` when you have them
  so the list matches faster and more accurately elsewhere.
