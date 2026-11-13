# CamelClip 0.16.0 — dependency-aware setup

Choose GUI, TUI, CLI or any combination. All installations require working yt-dlp, FFmpeg, ffprobe and Deno. Setup installs missing tools using Homebrew on macOS or the native Linux package manager with Homebrew fallback. Existing dependencies are reused.

Windows x64 and ARM64 setup uses WSL 2. It verifies the matching release archive before invoking Linux setup and creates Start menu shortcuts. See [setup and restart instructions](installers.md). This preview also includes clip creation, media companions, browser profile selection, grouped navigation and library fixes since 0.14.0.

The eight installer payloads are unsigned; macOS packages are not notarized. Native Windows end-to-end installation, interactive macOS system-wide provisioning, macOS Intel and Linux ARM64 acceptance remain pending. Use the release SHA256SUMS to verify downloads. Existing workspace settings and media are retained.
