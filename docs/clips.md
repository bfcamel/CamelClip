# Clip by Link

Clip by Link creates independently playable fragments from one video URL or an existing library asset. Domain rules and execution belong to the independent Core/application layer; GUI, TUI and CLI use the same persisted plans and commands.

**Availability:** the precise-processing increment is available in source/main and the locally updated application. The published v0.14.0 installers do not include it. No release or installer was rebuilt for this change.

## Supported workflow

- One finite recording per batch, 1–100 independent intervals.
- Millisecond bounds; each interval is at least one second. Overlaps are allowed; exact duplicate intervals are rejected.
- Manual ranges or available source chapters, resolved into immutable intervals during planning.
- Remote sources are acquired once per batch. A small fragment may require downloading the entire recording. Sources are private preparation data, not library downloads or automatic S3 uploads.
- Precise video output is re-encoded as H.264/AAC MP4. Audio-only output supports M4A, MP3, Opus, FLAC and WAV. Height selection never deliberately upscales the source.
- Full decode, selected-frame/sample evidence, frame counts, duration/profile checks, output hash and audio/video offset checks precede publication.
- Pause/resume, per-task or batch cancellation, explicit retries and restart recovery. Completed outputs are retained and are not rendered again when the worker restarts.
- Each published fragment appears in the library with an independent asset ID. Play and Reveal operate on the actual output file.
- Optional explicit S3 destinations. An empty selection remains local-only even if another profile has global AutoUpload enabled.
- Provenance export records original source identity, requested interval, observed boundaries, tool version and output checksum; it does not export browser cookies or private cache paths.

The current worker handles batches and fragments sequentially, sharing download/processing permits and rate budgets with ordinary downloads. This bounds resource use; it does not create an extra pool of FFmpeg workers. Compatible batches are not yet coalesced into a cross-batch source cache.

## GUI

Open **Clips → Create clips**, or choose **Create clips** in a library file's details. Select **Source URL** or **Library asset**, then provide the URL or asset ID. Existing library details show the asset ID.

Enter one interval per line:

```text
00:01:10.250..00:02:00 | Introduction
00:15:30..00:18:00 | Demonstration
```

Select the mode, quality, output directory and optional browser authentication. **Inspect chapters** loads available chapter selections without acquiring media. Selected chapters are added to manually entered intervals. Leave manual ranges empty to use only chapters.

Select S3 profiles only when delivery is wanted. **Review clips** shows the resolved source, intervals, destination folder and selected profiles. **Create batch** admits that reviewed plan. Keep CamelClip open while work runs; closing it preserves durable work for the next session.

The Clips screen shows shared source acquisition and per-fragment rendering/verification progress. A completed fragment has Play and Reveal actions. Details allow JSON provenance export. Failed or canceled work can be retried; **Clear source cache** releases private preparation data after a batch stops, without deleting published clips. Retrying after cleanup may require full acquisition again and must still match retained source evidence.

## TUI

Open screen **6 — Clips**. `n` opens the source/range/profile form, `p` pauses or resumes the selected batch, `x` cancels, `r` retries failed/canceled tasks, and Enter shows batch details.

Use `asset:ASSET_ID` for a library input. Separate multiple intervals with semicolons in the terminal form. Review the source, interval list and output directory before confirming. GUI and CLI offer source-chapter selection; the terminal form currently accepts manual intervals.

## CLI

Close another GUI/TUI owner before opening the same workspace from CLI.

```sh
# Metadata only; browser access occurs only when explicitly selected.
camelclip clips inspect --url 'https://www.youtube.com/watch?v=VIDEO_ID' --json

# Plan only: no media acquisition or rendering.
camelclip clips plan --url 'https://www.youtube.com/watch?v=VIDEO_ID' \
  --range '00:01:10.250..00:02:00 | Introduction' \
  --range '00:15:30..00:18:00 | Demonstration' \
  --out "$HOME/Downloads/CamelClip/Clips" --json

# Use an existing local asset instead of a URL.
camelclip clips plan --asset ASSET_ID --range '1.250..4.550 | Example' \
  --out "$HOME/Downloads/CamelClip/Clips" --json

# Admit exactly the reviewed plan, using fields from the plan response.
camelclip clips create --plan-id PLAN_ID --revision 1 \
  --digest PLAN_DIGEST --idempotency-key my-clip-request --json

# Execute only this batch; no unrelated queue or playlist work starts.
camelclip clips run BATCH_ID
camelclip clips list --limit 50 --json
camelclip clips items BATCH_ID --json
camelclip clips pause BATCH_ID --revision CONTROL_REVISION
camelclip clips resume BATCH_ID --revision CONTROL_REVISION
camelclip clips cancel BATCH_ID --revision CONTROL_REVISION --task TASK_ID
camelclip clips retry BATCH_ID --revision CONTROL_REVISION
camelclip clips provenance TASK_ID > clip-provenance.json
camelclip clips deliveries
camelclip clips cleanup BATCH_ID --revision CONTROL_REVISION
```

`create` persists work and exits; it does not create a daemon. Run the batch explicitly or open the GUI/TUI to execute pending work. Repeating `create` with the same key and plan digest returns the original batch. Reusing a key with different reviewed intent is rejected.

Additional planning flags: `--mode audio`, `--audio flac`, `--quality 720`, `--browser firefox`, `--profile PROFILE_ID,PROFILE_ID`, `--chapters 0,2`, and `--ranges-file ranges.txt`. Chapter indices come from `inspect`. The range text file uses the same syntax as the GUI and is limited to 1 MiB. `list` returns `next_cursor`; pass it with `--cursor` for another page. Results are JSON; command failures use existing CLI exit codes 1/2 and cancellation handling.

S3 jobs are persisted independently and wait for vault unlock. The one-shot clip worker admits selected deliveries but does not unlock a vault or keep running until uploads finish. Open the workspace and unlock the existing vault to transfer them. If a destination profile changed after review, the delivery is blocked; choose the current destination through the library's explicit upload action.

## Recovery and storage

SQLite schema **6** adds plans, batches, provenance and delivery intents without changing ordinary download IDs. Older binaries refuse the newer schema. Back up the workspace before upgrading; restoring a pre-upgrade backup is needed to return to an older executable.

Prepared sources live under the workspace's private `clip-cache` directory. Candidates are in a private `.camelclip-clips` staging directory on the destination filesystem. Publication uses a non-overwriting hard link and a durable `publishing` record. On restart, an interrupted publication verifies matching candidate/final identity and commits one library asset and delivery outbox. An unrelated occupied path is not overwritten.

Workers pass the staging lock to child processes. Another owner waits if an orphan child still holds it. Source snapshots, hashing and process I/O stream data; entire media files are not buffered in application memory.

Successful batches release their source cache. Failed/canceled batches retain private sources for explicit retry or cleanup. A 64 MiB filesystem free-space floor is checked at source/render stage entry; this initial increment does not yet provide continuous disk reservation accounting or the proposed automatic cache TTL/soft-budget manager. Reserve enough free space for the source and outputs before starting large batches.

## Deliberate boundaries

This is the **precise local/full-source increment**, not every item of the longer-term [specification](design/clip-by-link-specification.md).

Fast-copy with approved expanded bounds, partial/section acquisition, interactive local preview, per-task pending pause, cross-batch materialization reuse, automatic clip retries, cache TTL/soft-budget management, and continuous disk accounting remain future work. Unsupported accuracy modes are rejected rather than substituted. Finite known-duration media is required; live/DRM/unknown-duration sources, HDR, silent video, montage editing and subtitle slicing are unavailable.

Execution and fixture acceptance are tested locally on macOS arm64. CI includes Linux media tests, but a hosted check counts as passed only after it actually executes. External site/authentication behavior may change independently of local processing.
