Clipnaro 0.10.0 introduces the new product name, a guided first launch and a terminal selection fix.

- The executable is `clipnaro`, preferences use `Clipnaro` directories and `CLIPNARO_CONFIG_DIR` selects a separate profile.
- First launch explains the basic controls and offers language, output folder, video profile and optional browser cookies, followed by dependency/storage checks.
- Selected rows reset their background before the next cell style, keeping highlighting inside the correct panel.
- Deprecated CLI forms and JSON queue import have been removed. Use explicit `download`, `audio` and `formats` commands.

This pre-1.0 version targets macOS and Linux on arm64/amd64. Windows and a GUI are not supported. Dependencies are installed separately; builds are unsigned and macOS binaries are not notarized. Existing user data is handled during the maintainer's one-time installation update; the application contains no previous-brand compatibility paths.
