# MacDown

A small, terminal-only video and audio downloader for macOS, written in Go.
It uses [yt-dlp](https://github.com/yt-dlp/yt-dlp) for YouTube, VK and other supported sites, and FFmpeg to merge or convert media. Site support depends on your yt-dlp version and access to the particular video.

## Features

- Fetch the video's title, duration and available resolutions before downloading.
- Choose video with audio, or audio only.
- Navigate interactive menus with **↑ / ↓ and Enter**. Numbered menus are used when terminal controls are unavailable.
- Download one link or process a queue sequentially; a failed download does not stop subsequent links.
- Remember the output directory, mode, quality limit and audio format after successful downloads.
- Update download progress on the same terminal line.
- Show explanations for common access, network, format and disk errors.

**English and Russian interfaces are available.** The first-run default is detected from `LC_ALL`, `LC_MESSAGES`, `LANG`, then macOS preferred languages. Other languages fall back to English. Choose a language in the first menu or pass `-lang en`, `-lang ru`, or `-lang auto`. Your preference is saved after a successful download. yt-dlp diagnostics and operating-system errors may remain in their original language. There is no graphical interface or background server.

## Install and build

Install [Homebrew](https://brew.sh/) if needed, then:

```sh
brew install go yt-dlp ffmpeg deno
git clone https://github.com/bfcamel/video-downloader.git
cd video-downloader
go build -o macdown .
./macdown
```

Go 1.22 or newer is required to build. The Go program uses only the standard library. yt-dlp, FFmpeg/ffprobe and Deno must be available in your `PATH`; Go is not required to run a compiled binary.

For full YouTube support, yt-dlp also needs its EJS components. If your installation reports missing JavaScript components, follow the [official EJS setup instructions](https://github.com/yt-dlp/yt-dlp/wiki/EJS).

Build and check both Mac architectures:

```sh
sh build.sh
```

The `dist/` directory contains:

| File | Purpose |
| --- | --- |
| `macdown-macos-arm64` | Apple Silicon Macs |
| `macdown-macos-amd64` | Intel Macs |
| `Start.command` | Double-click in Finder to open the terminal interface |
| `README.md` | This guide |

You can also double-click the repository's `Start.command` after building. The launcher finds a binary next to itself or inside `dist/`. Compiled binaries are excluded from Git.

## Interactive workflow

Run `./macdown` without URLs:

1. Select a language, then paste one URL or several URLs separated by spaces on **one line**.
2. Select video with audio or audio only using the arrow keys and Enter.
3. For audio, choose the output format.
4. Enter an output directory, or press Enter to keep the saved value.
5. For each video, review its title and duration, then choose from the resolutions reported by the site.
6. The program downloads each item and prints a final success/failure count.

Russian menu labels (when Russian is selected):

| Russian | English |
| --- | --- |
| Видео со звуком | Video with audio |
| Только звук | Audio only |
| Лучшее доступное | Best available |
| До 1080p | Up to 1080p |
| Папка сохранения | Output directory |
| Сохранено | Saved |
| Ошибок | Errors |

Press **q** in an arrow-key menu to cancel, or **Ctrl+C** at any point to stop the queue. Terminal settings are restored when leaving a menu. Invalid numbered-menu input is prompted again.

## Command-line examples

Provide all flags **before positional URLs**. Alternatively, repeat `-url`.

Video with audio, up to 1080 pixels in height:

```sh
./macdown -mode video -quality 1080 -url 'https://www.youtube.com/watch?v=VIDEO_ID'
```

Best available quality:

```sh
./macdown -mode video -quality best -url 'https://vk.com/video-OWNER_ID_VIDEO_ID'
```

Audio only, converted to MP3:

```sh
./macdown -mode audio -audio-format mp3 -url 'VIDEO_URL'
```

Keep the source audio format when possible:

```sh
./macdown -mode audio -audio-format best -url 'VIDEO_URL'
```

Custom output directory:

```sh
./macdown -out "$HOME/Movies/Downloads" -url 'VIDEO_URL'
```

Multiple links with the same settings, without interactive prompts:

```sh
./macdown -mode video -quality 720 -url 'FIRST_URL' -url 'SECOND_URL'
```

Or:

```sh
./macdown -mode video -quality 720 'FIRST_URL' 'SECOND_URL'
```

Read links from a text file (one URL per line; empty lines and lines beginning with `#` are ignored):

```sh
./macdown -mode video -quality best -list links.txt
```

Inspect all yt-dlp formats without downloading:

```sh
./macdown -formats -url 'VIDEO_URL'
```

Use an existing browser login for a video that requires authentication:

```sh
./macdown -cookies-browser firefox -url 'VIDEO_URL'
```

Supported browser names: `safari`, `chrome`, `firefox`, `edge`, `brave`. Browser access is opt-in for each run and is **not saved**. macOS may ask for permission to read browser data. Cookies do not grant access to content your account cannot view.

Show flags: `./macdown -h`.

## Saved settings

On macOS, settings are stored in:

```text
~/Library/Application Support/MacDown/settings.json
```

The last successful download's language, mode, maximum height, audio format and directory become the next run's defaults, including command-line runs. Explicit flags override those defaults. URL lists and browser choices are never saved.

Rename or remove that file to reset the defaults. Missing settings use built-in defaults; malformed settings produce a warning and are ignored. Settings are written atomically with owner-only file permissions.

For a separate settings profile, set `MACDOWN_CONFIG_DIR` to a directory of your choice. Its `settings.json` will be used instead.

## Output and limitations

- Default output directory: `~/Downloads/MacDown`.
- Video is saved with audio in **MP4**, without forced re-encoding. The original codecs are retained: playback support (including QuickTime) depends on those codecs. If a source codec cannot be remuxed into MP4, the download reports an error rather than silently re-encoding.
- Quality is a **maximum source frame height**, not an exact resolution or an upscaling target. This also applies to portrait videos. If no suitable format with audio is available, the item fails.
- Audio formats: `mp3`, `m4a`, `opus`, `flac`, `wav`, `best`. Converting lossy audio to FLAC/WAV does not improve its original quality.
- File sizes are not estimated. Available resolutions are based on site metadata and may change before the download starts.
- Single-video URLs only: playlists, multi-video pages and ongoing livestreams are not processed by the download queue.
- Existing completed files are not overwritten. yt-dlp attempts to resume partial downloads when you repeat the same request. The queue itself is not persisted across runs.
- Browser authentication, regional restrictions and DRM are not bypassed.
- yt-dlp configuration files are ignored to keep behavior predictable.
- Exit status: `0` when all items succeed, `1` for errors, `130` when canceled.

## Troubleshooting

Update the download engine when sites change:

```sh
brew upgrade yt-dlp ffmpeg deno
```

| Problem | Suggested action |
| --- | --- |
| Missing yt-dlp / FFmpeg | Install dependencies using the command above |
| Sign-in, age restriction or private video | Check browser access, then pass `-cookies-browser` |
| HTTP 429 | Wait before retrying; the site is limiting requests |
| HTTP 403 | Update yt-dlp and check whether authentication is required |
| Connection or DNS failure | Check connectivity and retry |
| Requested quality unavailable | Select a different resolution or use `-quality best` |
| Not enough disk space / permission denied | Free space or choose a writable output directory |
| Corrupt settings | Rename or remove `settings.json` |

Unknown failures retain the original yt-dlp diagnostic. Metadata retrieval has a 90-second timeout. Failed items are reported and the remaining queue continues, unless you cancel.

## Source layout

| File | Responsibility |
| --- | --- |
| `main.go` | Entry point and exit status |
| `app.go` | Flags, workflow and sequential queue |
| `options.go` | Validation and yt-dlp arguments |
| `menu.go` | Arrow-key and numbered menus |
| `media.go` | Metadata, available qualities, child processes and readable errors |
| `progress.go` | In-place progress rendering and plain-text log output |
| `settings.go` | Loading and atomically saving preferences |
| `i18n.go` | Language detection and English/Russian messages |
| `*_test.go` | Unit tests |

Run checks:

```sh
go test ./...
go vet ./...
```

URLs are passed as individual process arguments, without a shell. On cancellation, the downloader's process group is stopped so its FFmpeg children do not continue working in the background.
