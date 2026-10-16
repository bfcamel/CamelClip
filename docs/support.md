# Supported environments and compatibility

| Platform | Architectures | Validation performed |
| --- | --- | --- |
| macOS | Apple Silicon arm64, Intel amd64 | Local arm64 tests; amd64 smoke/race tests under Rosetta |
| Linux | arm64, amd64 | Local arm64 Linux VM/container tests; amd64 smoke tests under emulation |
| Windows | — | Unsupported; requires additional OS adapters |

Other Linux distributions may work with the standalone CGO-free binary, but need separate validation before being included in the release support matrix. A POSIX terminal and `stty` are needed for the panel interface; pipes and `TERM=dumb` use numbered menus. SSH/headless environments support downloading. Opening files from the library requires a graphical session and xdg-utils on Linux.

Required runtime tools: yt-dlp and FFmpeg/ffprobe, with the selected encoders. Deno and yt-dlp EJS components are recommended for YouTube. External sites and authentication can change independently of MacDown; a successful doctor run does not guarantee access to a video.

The supported user interface consists of documented CLI commands/exit codes, settings and queue migrations. Internal Go interfaces are implementation boundaries, not a versioned public SDK. Exit codes: 0 success, 1 runtime/download/diagnostic failure, 2 invalid CLI usage, 130 canceled. Reports may contain WARN/INFO entries with exit 0; FAIL entries produce exit 1.

During 0.x development, documented behavior may change in minor releases. From 1.0, compatible fixes use patch releases, additions use minor releases, and incompatible public behavior requires a major release. Queue/settings migrations must preserve user data; schema downgrades are unsupported. Back up before an upgrade and keep the previous binary for rollback only when its database schema remains compatible.

Browser cookies are optional. Safari is available on macOS only. Linux Flatpak/Snap browser profiles may require an explicit profile directory; session/database permissions depend on the browser packaging. The application cannot grant access absent from your account.
