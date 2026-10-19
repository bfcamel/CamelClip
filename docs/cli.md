# CamelClip CLI reference

Verified against `internal/ui/terminal/cli.go` and `workspace_cli.go` on 2026-10-19.

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

`download URL...`, `audio URL...`, `formats URL...` accept positional URLs and `--list FILE`. Flags may precede or follow URLs; `--` ends option parsing. Download options: `--quality`, `--audio-format`, `--out`, `--cookies-browser`, `--video-codec`, `--video-format`. Limits: `--max-downloads`, `--max-processing`, `--rate-limit`, `--job-rate-limit`. `--lang auto|en|ru` selects the language. See README for values and fixed-share rate behavior.

CLI overrides apply to that process and do not save preferences. Download/audio/formats require URLs or a list. `tui` does not accept URLs.

## Queue, library, storage

These commands use a separate parser. Put `queue`, `library` or `storage` first. They do not accept general download flags or `--lang`; use saved preferences and `CAMELCLIP_CONFIG_DIR`. Use `camelclip queue --help`, not `camelclip --help queue`.

| Group | Actions |
| --- | --- |
| `queue` | `list`, `add URL...`, `priority ID low|normal|high`, `reorder ID up|down|first|last`, `pause`, `resume`, `retry`, `cancel`, `remove ID --yes` |
| `library` | `list`, `search TEXT`, `scan [DIR] [--recursive]`, `import FILE`, `refresh ID`, `remove ID --yes`, `delete-file ID --yes`, `show`, `open`, `reveal` |
| `storage` | `profiles`, `uploads`, `upload ASSET_ID --profile ID[,ID]`, `unlock`, `change-password`, `reset --yes` |

Queue actions accept comma-separated job IDs. Bulk results can contain per-item `Error` values even when the command exits zero; automation must inspect the result array. `queue add` only persists new work; open the TUI to execute it. `queue list` does not start downloads. `library remove` excludes an index entry without deleting its file; `delete-file` is a separate destructive action.

Library list/search flags include `--limit`, `--offset`, `--kind`, `--status`, `--container`, `--min-height`, `--sort`, `--since`, `--until`. `--json` is supported for relevant listing/detail/transfer commands; not every action emits JSON. Consult [S3 documentation](s3.md) for profile flags, private password descriptors, upload verification and confirmation requirements.

`storage unlock` applies only to the current foreground process. It cannot unlock a later command. `storage upload` unlocks and executes its own selected upload jobs. Secrets must be entered at a hidden TTY prompt or supplied through a private descriptor, never command arguments.
