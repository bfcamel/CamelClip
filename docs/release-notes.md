# CamelClip 0.13.0 — installer preview

Public preliminary release with the graphical workspace, camel-player branding and selectable GUI/TUI/CLI installers. TUI/CLI remain available; GUI uses the same local services and exclusive workspace lock. No new settings or database migration is introduced by the installer.

## Downloads

- macOS Apple Silicon / Intel: open the matching `camelclip_setup_0.13.0_macos_<arch>.pkg` and select GUI, TUI and/or CLI.
- Linux arm64 / amd64: extract `camelclip_setup_0.13.0_linux_<arch>.tar.gz`, enter the directory and run `./setup.sh`; for automation use `./setup.sh --components gui,tui,cli --yes`.
- macOS user-scoped archives are also provided; they do not require a system-wide package installation.
- Verify the downloaded files against `SHA256SUMS`.

See [installer instructions](installers.md) for updating, component removal, paths and prerequisites.

## Preview limitations

These installers are experimental, unsigned and not notarized by Apple. System-wide `.pkg` installation has not been accepted on a clean macOS system; native Linux and Intel runtime/desktop acceptance are pending. Linux GUI opens in a browser. The server component is not available. This preview is for evaluation, not a claim of production readiness.

Install yt-dlp, FFmpeg/ffprobe and, for relevant sites, Deno separately. Setup reports missing tools and does not silently install them or Homebrew. Existing `~/.local/bin/camelclip` can take precedence over a newly installed system command; check your PATH.

All seven component combinations, user-scoped native macOS installs/removals in temporary prefixes, archive integrity, ownership checks and read-only native Installer choices are tested. Cross-builds are not native acceptance tests. Settings, media, history and encrypted profiles are retained on removal. Each file is published atomically; multi-file installation is not crash-atomic. Review [recorded validation](installer-validation.md) before use.
