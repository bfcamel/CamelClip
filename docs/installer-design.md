# Component installer design

Status: component setup, user-scoped archives and native macOS .pkg choices are implemented. See [installer instructions](installers.md) for actual behavior, verification limits and remaining work (public signing/notarization, native Linux acceptance, distribution-specific packages and server support). This document records the broader design.

## Goal

A user downloads one installer for their OS and architecture, selects GUI, TUI and/or CLI, and receives ready-to-run software. The design must later support a server component without changing how local downloads, settings or history are stored.

The current install.sh installs only the combined terminal executable into ~/.local/bin. scripts/package.py packages only terminal releases. scripts/build_gui.sh builds the macOS app separately. These paths must converge on one release manifest and a tested component installation model.

## Component model

| Choice | User receives | Current runtime |
| --- | --- | --- |
| GUI | Application icon and graphical workspace | camelclip-gui and macOS WebKit host, or browser frontend on Linux |
| TUI | Terminal workspace launcher | camelclip tui |
| CLI | Shell commands and help | camelclip download/audio/queue/library/storage/etc. |
| Server (future) | Service/API deployment | Separate executable and configuration, not implemented yet |

The installer treats GUI/TUI/CLI as ways to access the product. TUI and CLI currently share one executable; checking both must not install two copies or charge disk space twice. Selecting only CLI does not promise removal of TUI code from that executable. Strict code-level separation is a separate product decision, not required for component installation.

The existing camelclip command and its documented behavior remain compatible. Add a dedicated TUI launcher instead of changing the meaning of existing commands. Hidden runtime payloads are resolved automatically; users never have to select an engine or database package.

At least one available user-facing component is required. Do not offer an installable server checkbox before the server exists.

## User journey

1. Welcome, language and detected OS/architecture.
2. Preset: Graphical (GUI), Terminal (TUI + CLI), or Custom.
3. Component checkboxes with clear descriptions, existing installation status and estimated size. The first desktop install defaults to GUI; subsequent runs preserve the installed selection.
4. Dependency check: yt-dlp, FFmpeg/ffprobe, Deno and relevant runtime support. Explain missing dependencies and the next action.
5. Review exact components, destinations and any requested privileges.
6. Install/update with clear progress and useful errors.
7. Finish: open GUI, open TUI, or show CLI examples according to the installed choices. Do not start a future server implicitly.

A graphical installation must not require Go, Swift, Python or compilation on the user's machine. Release builders produce the payloads in advance.

## Platform delivery

### First implementation: macOS

Produce a standard distribution .pkg using pkgbuild/productbuild. Use its Customize/component choices for GUI, TUI and CLI. Provide Apple Silicon and Intel builds initially; universal distribution can be added after native validation.

GUI destination: /Applications/CamelClip.app. This is distinct from the current development/source directory /Applications/CamelClip.

Keep shared terminal runtime files in a private, versioned product directory, e.g. /Library/Application Support/CamelClip/Runtime/<version>/. Component packages install distinct launcher paths and reference the shared runtime package. Two packages must never independently own and overwrite the same path. Do not add product-private directories to PATH.

CLI exposes /usr/local/bin/camelclip; the TUI choice adds a clearly named terminal launcher and a Finder-launchable shortcut. When both are installed they reference one runtime. Existing ~/.local/bin/camelclip installations must be detected: report PATH precedence, offer explicit migration, and never silently overwrite an unrelated executable or edit shell profiles.

For public distribution, sign the native application/executables with the appropriate Developer ID Application identity and the installer package with Developer ID Installer, then notarize the distribution. Signing credentials belong to the release process, not the repository. Development builds may be unsigned and must be labeled as such.

The standard package provides install/update/component selection. It is not a general maintenance/uninstall UI: deselecting an already installed package does not remove it. Build an explicit removal/maintenance action on top of the inventory model rather than presenting package deselection as uninstall.

### Linux and headless use

Keep the component IDs and release manifest identical. Provide an interactive terminal setup and explicit flags for noninteractive installs, then distribution-native packages as supported environments expand. Do not imply that a macOS .pkg is a cross-platform installer.

Default user-scoped locations can remain ~/.local/bin and ~/.local/share/CamelClip; add a desktop entry when GUI is selected. System-wide/service installation is a separate explicit mode.

## Dependencies

All current local interfaces use the same download/process tools. Installing GUI alone must still check those tools. Distinguish prerequisites for running the application from tools needed to build it.

For the initial installer, detect external dependencies and show concrete remediation, including a final report when a dependency is missing. Do not silently install Homebrew or run a package manager as root. Downloading tools requires an explicit choice and pinned, verified artifacts.

A later self-contained distribution can bundle suitable tool builds in a product-private directory. Before doing so, define architecture/OS support, update ownership, executable signing, redistribution licenses and notices for each actual build. Do not assume the application's Apache license covers dependency redistribution. Never modify an existing system FFmpeg or yt-dlp installation.

## Release and installation inventory

Use one release manifest for all installers. Minimum fields:

- schema version, product version, source commit, channel;
- OS, CPU architecture and minimum supported system version;
- stable component IDs and availability;
- payload filename, byte count, SHA-256 and component dependencies;
- package identifiers and required executable versions;
- compatibility requirements and data migration information.

Checksums detect corruption; release authenticity must come from a separately trusted signed manifest/distribution. Do not treat a checksum fetched beside an untrusted payload as proof of publisher identity.

Record an installation inventory separately from user preferences/queue data. It contains installed component IDs, payload versions, owned paths, hashes/links and installation scope. For macOS reconcile this with Installer receipts. Never store credentials, vault contents or browser selectors in the inventory.

## Updates and removal

- Preserve installed choices on upgrade; do not add GUI or a future server unexpectedly.
- Treat adding a component and removing one as explicit separate operations.
- Verify payloads before replacing executable files; stage and validate them first.
- Refuse an upgrade while the corresponding workspace is active, with a clear instruction to close it. Do not delete a stale-looking lock file or kill user downloads.
- Respect package-manager ownership. Do not let an in-app updater overwrite files owned by a system package installer.
- Keep a usable previous executable on failure where possible; do not claim multi-package .pkg installs are fully atomic.
- Back up data before schema migration. Restoring an older executable is not a safe database downgrade.
- Removal deletes only recorded product-owned files/links, after checking ownership and modifications.
- Preserve downloads, settings, queue/history and encrypted profiles by default. Erasing personal data is a separate, explicitly confirmed operation.
- Removing TUI must retain a terminal runtime still needed by CLI, and vice versa.

## Future server component

Installing a component and starting a service are different operations. The server has a separate config/state directory, identity and service lifecycle. Never point an always-running server at the local GUI/TUI database. The current exclusive-owner rule remains unchanged for local use.

A later remote client mode could let interfaces use the server API instead of opening local storage. That requires its own authentication, network exposure and compatibility design; it must not be implemented by weakening the existing lock.

Service startup, autostart, listening address and credentials are explicit setup steps. A server package must not enable an unauthenticated network listener as a side effect of installation. System service support is enabled per supported OS after acceptance testing.

## Implementation order

1. Define stable component/release manifests, terminal runtime ownership and inventory formats.
2. Extend release packaging to include the already-built GUI, terminal payloads and all notices; eliminate build-from-source requirements for installers.
3. Build the macOS .pkg with Graphical/Terminal/Custom choices and dependency reporting.
4. Validate install, upgrade and explicit removal workflows in isolated accounts/VMs, then sign/notarize for public release when credentials are available.
5. Add Linux terminal setup using the same manifest and component model.
6. Add a maintenance UI and a future server component once their underlying behavior exists.

## Acceptance criteria

Test each nonempty selection of GUI/TUI/CLI (seven combinations), clean install, same-version reinstall, upgrade preserving choices, adding/removing one component, missing dependencies, running-instance rejection, interrupted installation, modified/unowned paths, PATH conflicts, insufficient privileges and wrong architecture.

Verify that all selected entry points start, deselected entry points are not newly created, a shared runtime has one owner, uninstall leaves media and user data intact, and no installer reads browser cookies or unlocks encrypted profiles. Test production distribution on native macOS arm64/amd64 and supported Linux environments; a successful package build alone is insufficient.

## Primary references

- Apple Distribution XML and component choices: https://developer.apple.com/library/archive/documentation/DeveloperTools/Reference/DistributionDefinitionRef/Chapters/Distribution_XML_Ref.html
- Apple Installer JS: https://developer.apple.com/documentation/installer_js
- Developer ID signing: https://developer.apple.com/developer-id/
- Notarization: https://developer.apple.com/documentation/security/notarizing-macos-software-before-distribution
