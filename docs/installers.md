# Component installers

The current local build is **0.15.0**, including Clip by Link and poster/subtitle
workflows. It has not been published as a GitHub Release. The public download
links in README still point to the existing 0.14.0 preview.

CamelClip now has prebuilt component installers for macOS and Linux. Choose GUI, TUI and/or CLI. The unreleased server is rejected as a component, never installed or started. All interfaces keep the existing exclusive local workspace lock.

## macOS: standard graphical Installer

Open the package matching your Mac:

- Apple Silicon: `camelclip_setup_0.15.0_macos_arm64.pkg`
- Intel: `camelclip_setup_0.15.0_macos_amd64.pkg`

The standard Installer automatically presents component choices. GUI is selected for a clean install; existing installed selections are restored on later runs. Shared terminal files are selected automatically when TUI or CLI is chosen. GUI is installed as `/Applications/CamelClip.app`, TUI as `/Applications/CamelClip TUI.command` and `camelclip-tui`, CLI as `/usr/local/bin/camelclip`. Setup/maintenance is `/usr/local/bin/camelclip-setup`.

This is a system-wide installation and macOS asks for administrator authorization. It never edits shell startup files. If you used the older `~/.local/bin/camelclip` installer, check `command -v camelclip`: an older command earlier in PATH is not silently replaced.

Deselecting an existing package does not uninstall it. Explicit maintenance commands:

```sh
/usr/local/bin/camelclip-setup --status
sudo /usr/local/bin/camelclip-setup --remove gui --yes
sudo /usr/local/bin/camelclip-setup --uninstall --yes
```

Use the next `.pkg` for system updates. The setup engine refuses direct user-style updates of package-owned system files. Installer receipts are retained after explicit file removal; later installer defaults use actual component records rather than stale receipts.

These are local unsigned development packages. Public distribution still requires the app/executables and package to be signed with appropriate identities and notarized. Setting `INSTALLER_SIGN_ID` signs the outer package if the release builder has that identity; it does not sign/notarize the inner app automatically. Never disable Gatekeeper as part of installation.

## Linux: interactive or unattended user setup

Download/extract the matching `camelclip_setup_0.15.0_linux_amd64.tar.gz` or `camelclip_setup_0.15.0_linux_arm64.tar.gz`, enter the extracted directory and run:

```sh
./setup.sh
```

The menu offers GUI, TUI+CLI, all components, custom selection, update of installed components and removal. All installer prompts are in English. For automation:

```sh
./setup.sh --components gui,tui,cli --yes
./setup.sh --components cli --yes
./setup.sh --components tui,cli --yes
```

Install for the current user without sudo. Default locations:

- `~/.local/bin`: selected launchers and `camelclip-setup`;
- `~/.local/share/CamelClip`: private runtime, notices and installation inventory;
- `~/.local/share/applications`: selected desktop menu entries;
- `~/.local/share/pixmaps`: GUI logo.

TUI and CLI share one runtime. GUI opens the local interface in the default browser; it is not a separate native Linux window. GUI needs a desktop/browser; CLI/TUI also work through SSH. A terminal desktop entry requires a desktop environment with support for `Terminal=true`.

Put `~/.local/bin` on PATH if your shell does not already do so; setup prints that instruction and does not alter shell profiles.

```sh
camelclip-setup --status
camelclip-setup --remove tui --yes
camelclip-setup --uninstall --yes
```

For a custom prefix:

```sh
./setup.sh --prefix /absolute/path --components cli --yes
/absolute/path/bin/camelclip-setup --status
```

The installed maintenance launcher remembers its prefix. Custom desktop/icon directories may need explicit integration with your desktop's XDG search paths. Paths containing spaces/apostrophes are supported; symlinked destinations inside the installation are refused.

An additional macOS `.tar.gz` provides the same user-scoped setup without a system package: its GUI goes to `~/Applications/CamelClip.app` (or `<prefix>/Applications` with an explicit prefix).

## Updates, prerequisites and retained data

For user-scoped updates, extract the next setup archive and run `./setup.sh --components cli --yes` (or any chosen component). Existing installed choices are retained and updated together. Removing a component is always explicit; removing CLI keeps the shared terminal runtime if TUI still needs it.

All modes need external yt-dlp and FFmpeg/ffprobe for downloads; Deno is recommended for YouTube. User setup reports missing tools before installation, or fails with `--require-dependencies`. The macOS package records detection results at `/Library/Application Support/CamelClip/installation/dependencies.txt`. No installer downloads tools, installs Homebrew or invokes a system package manager silently. Running the installers requires neither Go, Swift nor Python on the target machine.

Close running GUI/TUI/CLI sessions before updating/removing. User setup checks the default queue lock, `CAMELCLIP_CONFIG_DIR`, or the directory passed with `--config-dir`. It cannot discover arbitrary custom workspaces in unrelated processes; pass their configuration path and close all sessions. The macOS system package also checks running executable names across accounts before changing files.

Settings, queue/history, encrypted credentials and media are outside the installation inventory and are retained on removal. Setup verifies file ownership and checksums before replacement/deletion and stops on unowned/modified paths. Back up modified installed files before repairing an installation. Do not use removal as a credential/data erasure tool.

Ordinary write failures roll back changed files and metadata. File publication is atomic per file; a whole multi-file or multi-package installation is not crash-atomic. After power loss, inspect partial installations and restore verified payloads; do not reset a queue to fix application installation. Data migrations remain the application's responsibility; executable downgrades are refused.

## Build and verify

Release builders need Go, Python 3, and macOS SDK/Swift/pkgbuild/productbuild for native Mac artifacts. Target users do not.

```sh
sh scripts/build_installers.sh                  # all four targets; run on macOS
sh scripts/build_installers.sh linux amd64     # one target
sh scripts/build_installers.sh darwin arm64    # Mac package and user archive
python3 scripts/test_installers.py             # after all four builds
python3 scripts/test_installer_choices.py dist/installers/0.15.0/camelclip_setup_0.15.0_macos_arm64.pkg  # read-only native choice evaluation
```

Outputs are in `dist/installers/0.15.0/`: two macOS `.pkg` files, four user-setup `.tar.gz` archives and `SHA256SUMS`. Debug Distribution XML files are also emitted. Existing terminal-only release packaging and `install.sh` remain available for compatibility.

Each user archive has a schema-1 manifest with stable component IDs, OS/architecture, version/commit and every payload file's hash, size, destination and mode. Builds outside a clean release checkout default to `development-local`; release builders supply `RELEASE_COMMIT` and `SOURCE_DATE_EPOCH` after validating their exact source revision. Packaging does not claim to validate source provenance automatically.

Verify `SHA256SUMS` before extraction. Checksums detect corruption, not publisher identity; obtain packages/manifests from a trusted source. Published versioned assets must not be overwritten.

Tests cover all seven choices, native launcher execution, reinstall/add/remove/uninstall, prefix quoting, modified/unowned files, symlink rejection, wrong architecture, corrupt payloads, workspace lock and rollback. Package tests inspect both native Mac payloads without installing them. Cross-built Linux and Intel payload checks do not replace acceptance testing on those operating systems/hardware.

## Installer language and source layout

Setup, distribution metadata and project documentation use English. English and Russian localization remain in the application only. Source components are in `cmd/camelclip-setup` and `internal/installer`; package assembly is in `scripts/package_setup.py`. Intermediate binaries/app bundles are generated under `dist/build/`, and downloadable artifacts are grouped under `dist/installers/<version>/`.
