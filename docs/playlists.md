# YouTube playlist subscriptions

CamelClip 0.14.0 adds persistent playlist monitoring to the shared Core. It requires no AI models, YouTube Data API key or extra service. It uses the installed yt-dlp to stream playlist entries and existing download/verification/S3 adapters for execution.

## Add and monitor

GUI: open **Playlists**, choose **Add playlist**, select an initial mode, output profile, interval and optional S3 profiles. TUI: press **5**, **n** to add, **e** to edit, **p** to pause/resume, **s** to check soon, **b** to backfill, or **Enter** for item status. TUI S3 selection uses comma-separated existing profile names.

```sh
camelclip playlists add 'https://www.youtube.com/playlist?list=PLAYLIST_ID' \
  --name 'My archive' --initial new_only --interval 15 --quality 1080
camelclip playlists list --json
camelclip playlists items SUBSCRIPTION_ID --limit 50 --offset 0
camelclip playlists sync SUBSCRIPTION_ID
camelclip playlists watch
```

`sync` performs one discovery scan without running transfers. `watch` runs continuous discovery, bounded admission and downloads until Ctrl+C. Close other CamelClip sessions before using CLI: all local interfaces share one owner lock.

```sh
camelclip playlists add 'PLAYLIST_URL' --initial all_existing --profile PROFILE_ID
camelclip playlists edit SUBSCRIPTION_ID --interval 30 --profile PROFILE_A,PROFILE_B
camelclip playlists edit SUBSCRIPTION_ID --profile ''    # future items local-only
camelclip playlists pause SUBSCRIPTION_ID
camelclip playlists resume SUBSCRIPTION_ID
camelclip playlists backfill SUBSCRIPTION_ID --yes
camelclip playlists retry SUBSCRIPTION_ID VIDEO_ID
camelclip playlists watch --password-fd 3 3</private/password-file
```

Password files must have owner-only permissions. No password is passed in command arguments or saved in subscription settings. When credentials remain locked, local downloading can continue subject to disk limits while cloud delivery waits. Unlock the vault in GUI/TUI, or provide a private password descriptor to CLI watch.

## Identity and initial mode

One subscription follows one canonical playlist ID. Standard playlist URLs and watch URLs with a list parameter resolve to the same identity. Generated mixes and unsupported context lists are rejected.

- **new_only:** first complete enumeration establishes the baseline, without downloading those entries.
- **all_existing:** existing entries and future discoveries are eligible.
- **Backfill:** explicitly makes baseline entries eligible later.

The original initial mode and playlist identity cannot be edited; use backfill for existing history. A failed/partial initial scan does not establish a baseline. The baseline covers entries observed during the first successful enumeration, not an atomic snapshot of YouTube at the instant Add was pressed.

New means a video ID not previously observed in that subscription. Adding an older video is new; moving, renaming, duplicating or removing/re-adding a known video is not. Full scans discover insertions anywhere. Private/deleted/unavailable entries retain an ID when yt-dlp exposes one; failed downloads are not repeatedly re-admitted by polling. Retry failed/canceled jobs explicitly from playlist details or the regular queue. After removing a failed job's queue history, it cannot be retried through that deleted job; retained subscription history still prevents silent resurrection.

## Local files and S3

Compatible subscriptions share a materialization keyed by video, normalized output profile and canonical output folder. Different folders or profiles deliberately produce separate materializations. Queue-history removal keeps fulfillment history and does not redownload completed files. Manual file deletion does not opt an item back into automatic download.

A subscription with no selected profiles is strictly local-only and bypasses global AutoUpload. Selected profiles produce destination snapshots and durable delivery bindings. A later subscription can request S3 for an already completed compatible local file without downloading it again. Destinations use `<profile-prefix>/youtube/<video-id>/<materialization-hash>.<extension>`; remote titles and playlist order are not object keys.

Profile destination changes block existing pending uploads until explicitly accepted in the upload queue. Changing subscription settings affects undispatched items; already admitted jobs and delivery obligations retain their captured settings. A missing destination blocks admission for the affected item, leaving other subscriptions operational; edit the subscription to select an available profile or local-only mode. No local file is automatically deleted after upload.

Pausing prevents new discovery/admission and cancels an in-flight scan. Already admitted downloads and uploads continue, since another subscription may share them. Cancel those explicitly in the corresponding queue. Resume catches up once rather than replaying every missed polling interval.

## Capacity and recovery

There is no subscription-count cap. SQLite queries are paginated, enumeration is streamed in batches of at most 250, and two scan workers are shared across subscriptions. Admission is fair across playlists with at most ten items per scheduler tick and a backlog ceiling of 1,000 nonterminal download jobs. Additional discoveries remain durable in SQLite until capacity is available. Download/processing/disk policies remain shared with manual work.

The minimum interval is five minutes; the default is fifteen. Backoff applies to failed scans. Long playlists can occupy scan slots and increase actual detection latency; intervals are requested schedules, not deadlines. A video appearing and disappearing between polls may never be observed. yt-dlp and YouTube changes may require dependency updates.

Subscriptions, baselines, queue admission keys, source bindings and delivery records survive restart. Interrupted scans are replayed, partial baselines are discarded, and admission/publication/delivery transactions are idempotent. Database schema 5 migrates older supported workspaces transactionally; older executables cannot open the migrated workspace. Back up application data before downgrading or replacing it.

This preview does not install a daemon/server. Monitoring runs only while GUI/TUI or CLI watch is running. A server edition remains future work.
