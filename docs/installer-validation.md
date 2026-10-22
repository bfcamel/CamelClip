# Installer validation — 2026-10-22

Public preview candidate: CamelClip 0.13.0. Release artifacts must be rebuilt from the final merged source commit. No earlier published assets are replaced.

| Check | Result |
| --- | --- |
| Full Go suite with race detection and go vet | Passed |
| Installer unit tests, all seven component combinations | Passed |
| Modified/unowned paths, symlink/traversal rejection, wrong architecture | Passed |
| Hash rejection, running workspace lock and failed upgrade rollback | Passed |
| Four prebuilt user-setup archives, per-file hashes and executable architecture | Passed |
| Native macOS arm64 user-scoped installation in temporary prefixes | Passed for all seven combinations |
| Native installed CLI execution, repeated install, add/remove/uninstall, preserved test data | Passed |
| Interactive bilingual menu through a real PTY | Passed |
| Both macOS .pkg payloads and component ownership records | Expanded and inspected; passed |
| macOS Installer default selection and seven dependency combinations | Read-only evaluation passed |
| System-wide .pkg installation into /Applications and /usr/local | Not performed; requires isolated system/account acceptance |
| Linux arm64/amd64 runtime and desktop integration | Cross-built and artifact-validated; native Linux acceptance pending |
| Intel macOS runtime | Cross-built and artifact-validated; native Intel acceptance pending |
| Public code signing and notarization | Not performed; local packages are unsigned |

User installation tests used temporary configuration/prefix directories and disposable fixture preferences. No real browser profiles, vault credentials, queue databases or downloaded user media were accessed by installer tests. macOS choice validation used only `-showChoicesXML` and `-showChoicesAfterApplyingChangesXML`; neither installs a package.

Build all targets with `sh scripts/build_installers.sh`. See [installer instructions](installers.md) for verification commands and release requirements.
