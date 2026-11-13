# Troubleshooting

Start with `camelclip doctor`. For an issue attachment, use:

```sh
camelclip doctor --json > doctor.json
```

Doctor performs offline checks and temporary write probes in existing directories. It does not start downloads, read cookies, modify settings, repair databases or validate site login. JSON output excludes usernames, paths, URLs, browser-profile selectors and raw tool output; it retains platform, application/Go/tool versions, selected codecs and whether cookies are enabled. Messages in the structured report currently use English. Review other logs yourself: normal download logs can contain URLs and local paths.

- **Missing tool or codec:** install the listed dependency or a full FFmpeg build, then rerun doctor. Select another codec if that encoder is unavailable.
- **Login or bot challenge:** update yt-dlp and its EJS components; use the exact Firefox/Chrome profile where the video opens and the same network. Pass `--cookies-browser firefox:PROFILE`. Browser access is opt-in. Close the browser if its database is locked. Flatpak/Snap profiles may require a full path. Doctor does not confirm the session is authenticated.
- **Disk space:** processing keeps the source and converted result temporarily. Free enough space for both and retry the failed job. Doctor warns below 512 MiB; that threshold does not estimate the size of a particular video.
- **File does not play:** inspect the selected codec/container and your player support. CamelClip verifies stream presence, duration and complete decoding before publication. It preserves existing files rather than replacing them. Share the doctor report and exact error, with private values removed.
- **Queue already open:** close the other CamelClip instance. Do not remove queue.lock while a process is using it; locks are released by the OS when the owner exits.
- **Damaged queue:** close CamelClip and copy the entire configuration directory, including queue.sqlite3, queue.sqlite3-wal and queue.sqlite3-shm if present. Keep the original untouched. Recovery should be attempted on a copy using SQLite tooling or with maintainer assistance. Automatic repair is intentionally not provided; do not delete the database to make the error disappear.
- **WSL browser or Explorer fails to open:** run `powershell.exe -NoProfile -Command Get-Location` and `wslpath -w "$HOME"` in the same distribution. Windows interoperability and Windows PATH integration must be enabled. Use `camelclip-gui --no-open` to open the printed private URL in the Windows browser manually.
- **Native Linux file opening fails:** install xdg-utils and use a graphical desktop session. CLI downloads work without a desktop. `o` opens the containing folder; selecting an exact file is desktop-dependent.

Configuration is in `~/Library/Application Support/CamelClip` on macOS and `${XDG_CONFIG_HOME:-$HOME/.config}/CamelClip` on Linux. `CAMELCLIP_CONFIG_DIR` overrides both settings and queue locations. It can isolate a reproduction without touching the real queue:

```sh
CAMELCLIP_CONFIG_DIR=/absolute/path/to/test-config camelclip
```
