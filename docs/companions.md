# Posters and subtitles

Available in source/main builds. Existing release installers are unchanged.

CamelClip can discover existing subtitle tracks, load a poster, search a saved
transcript and propose a time range for Clip by Link. The application does not
transcribe, translate or generate subtitles.

## Desktop

In **Create clips**, enter a URL or library asset, choose browser cookies if
needed, and click **Poster and subtitles**. Discovery reads metadata; it does not
download the full video. Click **Load poster** or select a caption track and click
**Load selected captions**. Manual, automatic, embedded and translated tracks are
identified separately. Automatic captions may contain recognition errors.

Search the transcript, select the first and last cues, then click **Add reviewed
range**. The range is appended to the clip form. Review it through the usual
Clip by Link confirmation before creating a batch. Overlapping cues are retained;
the proposed range spans their complete time envelope. Invalid or out-of-source
ranges are rejected rather than silently clamped.

Library file details also offer **Poster and subtitles** for local media.
Historical library rows do not trigger network requests. Closing the panel releases
its poster object URL. All companion API requests use the existing desktop session
authorization; cached files are addressed by opaque IDs rather than public paths.

## Terminal

In the library or clips screen, press **h**, enter a URL or `asset:ID`, then enter
the track index shown in the session log. Enter `poster` instead to cache the
poster. In transcript details, **s** searches the first 100 matching cues and **r**
accepts a zero-based `FIRST:LAST` cue selection, then opens the normal clip review
workflow. CLI pagination provides access to the complete transcript.

```sh
camelclip companions inspect --url 'VIDEO_URL'
camelclip companions inspect --asset ASSET_ID
camelclip companions poster --id CATALOG_ID
camelclip companions caption --id CATALOG_ID --track TRACK_ID
camelclip companions caption --url 'VIDEO_URL' --language en --browser firefox
camelclip companions cues --id SNAPSHOT_ID --query 'search text' --limit 100
camelclip companions cues --id SNAPSHOT_ID --cursor 100
camelclip companions range --id SNAPSHOT_ID --first 3 --last 8
camelclip companions import --id CATALOG_ID --file subtitles.srt --language en
camelclip companions export --id SNAPSHOT_ID --file new-subtitles.vtt
camelclip companions links --owner ASSET_ID
camelclip companions keep --owner ASSET_ID --id BLOB_ID --keep=false
camelclip companions cache --prune
```

Commands return JSON and share the existing workspace owner lock. Catalogs expire
after six hours. Browser-authorized catalogs require discovery again in a new
process: use the single-command URL caption fetch above or the desktop workflow.
Already saved snapshots and posters remain readable without cookies or a network.

Imported SRT/WebVTT files are explicitly selected, never found by scanning adjacent
files. Their clock is unverified; transcript range proposals are disabled for
imports. Review their times manually. WebVTT timestamp-map streams are rejected
until explicit clock mapping is supported. Export produces normalized plain-text
WebVTT, retaining raw bytes in the workspace, and refuses to overwrite a file.

## Storage and limits

The existing workspace database migrates transactionally from schema 6 to 7 without
modifying downloads, library media, clip batches or upload jobs. It stores source
catalogs, immutable caption snapshots, blob metadata and separate cached/kept
references. Private lookup requests and account scope are excluded from public
JSON. Cache files live under the application data directory:

```text
companions/blobs/<scope>/<kind>/<digest-prefix>/<content-digest>.<extension>
```

Files use private permissions and content digests. Reads verify their byte count
and SHA-256. Captions retain the original file and a normalized cue snapshot;
HTML styling is reduced to safe plain text, preserving overlap and repeated
speech. Limits: 20 MiB raw captions, 100,000 cues, 16 KiB per cue, 32 MiB normalized
snapshot, 200 discovered tracks, 500 cues per API page and 512-byte search queries.
Poster output is normalized to JPEG at at most 640 pixels per side. Source image
resolution is capped at 20 megapixels; output is capped at 8 MiB.

The cache defaults to 512 MiB and seven days. Cleanup runs before companion fetch
operations and explicitly through `cache --prune`. A fetch may temporarily exceed
the budget until the next cleanup. Kept references are excluded from eviction and
reported separately. For a saved local asset, one poster and up to two raw/parsed
caption pairs per kind are retained automatically; explicit `keep`/unkeep is also
available. Fresh catalogs use isolated scopes, including a new scope for each
browser authorization session. A missing/corrupt cache entry is an error, not a
claim that the source has no subtitles. Files are read into bounded buffers under
the service lock so cache cleanup cannot race an active read.

## Implementation boundary

This increment implements poster and transcript workflows (specification stages
1–3), plus explicit import/export and transcript-to-clip handoff. It does **not**
include an embedded video player, playback source/proxy preparation, storyboards,
automatic clip subtitle trimming/export, S3 companion delivery, custom cache
settings, background refresh or cache orphan reconciliation. Current S3 behavior
continues to upload the selected media files only. The remaining stages are
specified in [the full design](design/media-companions-specification.md).
