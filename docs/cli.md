# CamelClip CLI reference

The CLI shares the application services with GUI and TUI. Run `camelclip help` for command-specific help.

## Launch and diagnostics

| Command | Behavior |
| --- | --- |
| `camelclip` / `camelclip tui` | Guided first run when settings are absent, then terminal workspace |
| `camelclip setup` | Run guided setup again |
| `camelclip config` | Effective preferences and settings path as JSON |
| `camelclip doctor --json` | Offline dependency/storage report; does not test site authentication |
| `camelclip version` / `camelclip --version` | Application version |
| `camelclip help` / `camelclip help download` | Download/general help |
| `camelclip queue --help` | Workspace command help (also library/storage) |

`Start.command` is the macOS Finder launcher. Terminal users can run `camelclip tui` from any directory after installation. `CAMELCLIP_CONFIG_DIR` overrides the configuration directory for every command. Do not run two processes against the same queue; a process lock enforces a single owner.

## Downloads

`download URL...`, `audio URL...`, `formats URL...` accept positional URLs and `--list FILE`. Flags may precede or follow URLs; `--` ends option parsing. Download options: `--quality`, `--audio-format`, `--out`, `--cookies-browser`, `--video-codec`, `--video-format`. Limits: `--max-downloads`, `--max-processing`, `--rate-limit`, `--job-rate-limit`. `--lang auto|en|ru` selects the language. See the format and rate reference below.

CLI overrides apply to that process and do not save preferences. Download/audio/formats require URLs or a list. `tui` does not accept URLs.

## Queue, library, storage

These commands use a separate parser. Put `queue`, `library` or `storage` first. They do not accept general download flags or `--lang`; use saved preferences and `CAMELCLIP_CONFIG_DIR`. Use `camelclip queue --help`, not `camelclip --help queue`.

| Group | Actions |
| --- | --- |
| `queue` | `list`, `add URL...`, `priority ID low|normal|high`, `reorder ID up|down|first|last`, `pause`, `resume`, `retry`, `cancel`, `remove ID --yes` |
| `library` | `list`, `search TEXT`, `scan [DIR] [--recursive]`, `import FILE`, `refresh ID`, `delete-file ID --yes`, `show`, `open`, `reveal` |
| `storage` | `profiles`, `uploads`, `upload ASSET_ID --profile ID[,ID]`, `unlock`, `change-password`, `reset --yes` |

Queue actions accept comma-separated job IDs. Bulk results include per-item errors; actions exit nonzero on partial failure. Automation should inspect the full result array. `queue add` only persists new work; open the TUI to execute it. `queue list` does not start downloads. `library refresh` reports a missing file as missing. Confirmed `delete-file` removes its library entry and deletes the local file if present.

Library list/search flags include `--limit`, `--offset`, `--kind`, `--status`, `--container`, `--min-height`, `--sort`, `--since`, `--until`. `--json` is supported for relevant listing/detail/transfer commands; not every action emits JSON. Consult [S3 documentation](s3.md) for profile flags, private password descriptors, upload verification and confirmation requirements.

`storage unlock` applies only to the current foreground process. It cannot unlock a later command. `storage upload` unlocks and executes its own selected upload jobs. Secrets must be entered at a hidden TTY prompt or supplied through a private descriptor, never command arguments.

### Queue action exit status

`queue add` and bulk queue actions print the complete JSON result array, including successful items. They exit nonzero if any item contains an `error`; scripts must not treat partial success as total success. Successful items are retained and are not rolled back.

## Formats and rate limits

Video codecs: `h264` (default), `hevc`, `vp9`, `av1`. Containers: `mp4` (default), `mkv`, and `webm` for VP9/AV1. Audio-only formats: `mp3`, `m4a`, `opus`, `flac`, `wav`, `best`. Each queued request captures its own settings.

Download and processing slots are independent. Defaults are five downloads and `max(cpu_count / 4, 2)` processing tasks. Override with `--max-downloads 1..32` and `--max-processing auto|1..32`.

```sh
camelclip download --list links.txt --rate-limit 2MiB/s
camelclip download 'VIDEO_URL' --job-rate-limit 512KiB/s
camelclip download 'VIDEO_URL' --max-downloads 1 --rate-limit 2MiB/s
camelclip download 'VIDEO_URL' --rate-limit 0 --job-rate-limit 0
```

Rates are bytes/second; `1MiB/s` is 1,048,576 B/s. Zero disables a limit. The global budget uses fixed shares based on maximum download concurrency, so unused shares are not redistributed. With both limits configured, the smaller nonzero global share or per-job cap applies. Changes affect new processes; running transfers keep their existing limits. CLI overrides do not change saved preferences. S3 upload rates are separate.

TUI Settings expose the same total-budget and per-job controls. The application supports English and Russian UI labels; CLI documentation is English-only.

## Playlist subscriptions

`camelclip playlists add|edit|list|items|pause|resume|sync|backfill|retry|watch` manages persistent subscriptions. One-shot commands do not start background media transfers. `watch` explicitly runs monitoring/downloads; S3 credentials can be unlocked with `--password-fd`. See [complete playlist behavior and examples](playlists.md).

## Clip commands (source/main)

`camelclip clips --help` describes precise fragment planning, admission, execution, listing, control and provenance export. Plans and idempotency keys separate review from execution; a one-shot command does not start a daemon. See [complete examples and limitations](clips.md). Published v0.14.0 installers are unchanged.

## Media companions (source/main)

[Posters and subtitles](companions.md) use an interface-independent application
service, scoped content-addressed files and immutable caption snapshots. See the
linked guide for commands, desktop/terminal workflows, limits and remaining stages.
