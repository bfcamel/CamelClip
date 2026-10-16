# Roadmap to 1.0.0

The 1.0 scope is a reliable local terminal downloader for macOS and Linux. The isolated downloader, download/process/verify pipeline, SQLite queue, separate JSON preferences, codec profiles, diagnostics and Apache-2.0 license already exist. The release process is local and does not require GitHub Actions.

## Required before 1.0

| Priority | Work | Acceptance evidence |
| --- | --- | --- |
| P0 | Select the final product name and update executable/package names, documentation and repository branding | Resolve the existing MacDown name overlap; if renamed, migrate old config/output paths without moving or losing user data |
| P0 | Define the supported OS/version/architecture matrix and external dependency baseline | Record actual macOS/Linux versions and yt-dlp/FFmpeg/Deno versions; run the local suite and extracted-package smoke tests on each supported target; record emulation separately |
| P0 | Validate clean installation, update, removal and compatible rollback | A new user follows README on macOS and Linux; upgrade from 0.8/0.9 retains settings, queue and history; uninstall preserves media/data; rollback instructions match schema compatibility |
| P0 | Add a user-facing, consistent queue backup/restore procedure | Back up a live SQLite queue consistently, restore into an isolated directory, retain jobs/settings/history; refuse damaged or incompatible backups without replacing working data |
| P0 | Close remaining real-world failure scenarios | Record behavior for a real transfer interruption, actual low/full output storage, browser-session expiry, missing encoders, permission loss and duplicate concurrent URLs; no unverified publication or silent queue reset |
| P0 | Freeze the documented public behavior | Document CLI flags, exit codes, settings defaults, migration rules, retry/cancel/restart semantics and supported limitations; audit implementation against the docs |
| P0 | Run external release-candidate acceptance | Independent users on both OS families install and use the candidate; triage all reports; resolve every reproducible crash, data-loss, queue-corruption and invalid-output issue before release |
| P0 | Produce a traceable 1.0 package and release record | Exact source commit, version, four archives, license notices, verified SHA256SUMS, changelog and completed validation record; maintainer publishes the reviewed draft |
| P1 | Finish onboarding and diagnostic wording | A new user can identify and resolve missing dependencies, unsupported codecs, headless opening and cookie-profile problems using the UI/doctor/docs; no credentials in the shareable report |
| P1 | Review macOS trust and distribution | Choose the supported install path; test download/quarantine behavior on a clean Mac; use signing/notarization if distributing a signed app, or document the tested unsigned-binary installation accurately |

P0 items block release. P1 items must be reviewed during the release candidate and either completed or explicitly accepted as documented limitations. No fixed date or stability percentage is assumed.

## Sequence

1. **Stabilize 0.9:** settle naming/support boundaries, add the backup/restore procedure and complete missing failure tests. Keep this work focused on the existing terminal product.
2. **1.0.0-rc.1:** produce an installable candidate, run independent acceptance on macOS/Linux and collect a written validation record. Fix reported issues and repeat affected checks; use another candidate if needed.
3. **1.0.0:** publish when all P0 criteria pass and remaining limitations are explicit. Preserve released artifacts and maintain patch releases for regressions.

## Release acceptance record

Copy this checklist into the candidate's release notes and fill in results; unchecked items remain outstanding.

- [ ] Final name and migration behavior accepted.
- [ ] Supported OS versions, architectures and dependency versions listed.
- [ ] Formatting, race tests, vet and media tests passed with no required tests skipped.
- [ ] Native smoke results recorded; emulated results identified separately.
- [ ] Clean install, upgrade from prior versions, uninstall and rollback checked.
- [ ] Backup and isolated restore verified; damaged backup safely rejected.
- [ ] Remaining real-world failure scenarios recorded and passed.
- [ ] CLI/settings/migration documentation reviewed against actual behavior.
- [ ] External candidate feedback triaged; no unresolved release-blocking defects.
- [ ] Four archives, build metadata, notices and checksums verified.
- [ ] Draft reviewed and publication approved by the maintainer.

## After 1.0

A GUI, Windows, playlists, pause/resume controls, Homebrew or distribution-specific packages, themes, an external SDK and hosted CI can be planned independently. They do not block the terminal 1.0 scope. The existing core/application interfaces remain the integration boundary for future frontends.
