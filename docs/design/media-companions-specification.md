# Media Preview and Subtitles — Product and Core Specification

- **Status:** Proposed; not implemented.
- **Version:** 1.0.
- **Date:** 2026-10-23.
- **Inspected baseline:** main `1b6621466f0fe27813b4c0778b1b56cdf71b6e8f`, including the precise clip increment and SQLite schema 6.
- **Scope:** Covers optional visual companions, source playback and existing subtitle tracks for ordinary downloads, the library and Clip by Link. It does not authorize implementation, publishing, installers or application updates.
- **Normative terms:** MUST/MUST NOT are requirements; SHOULD is the default unless an explicit documented reason justifies an exception.

## 1. Product decision

CamelClip SHOULD support previews and existing subtitles because they solve concrete tasks: recognize a recording, find a phrase, choose an interval and check a cut. It SHOULD NOT automatically download every language, full video proxies or all storyboard images for every queued URL.

The recommended model is **small companions on demand, durable selected companions with saved media, explicit acquisition for playback**. Cache data and user-kept data have different retention contracts even if their bytes share a content-addressed store.

| Material | Default acquisition | Default retention | Main value |
| --- | --- | --- | --- |
| Provider poster/thumbnail | When a visible item needs it, after source inspection | Bounded cache; keep the selected poster with a saved asset | Recognize videos in lists |
| Timeline thumbnails/storyboard | On explicit timeline use, if supported | Disposable cache | Navigate without repeatedly seeking video |
| Subtitle track catalog | Alongside an already authorized metadata inspection | Metadata cache, including a time-bounded absence result | Show languages and origin |
| Selected subtitle content | When the user opens the transcript/enables captions | Cache initially; keep already loaded selected tracks with saved media according to policy | Read, search, choose bounds |
| Playable source/proxy | When the user presses Prepare preview/Play | Leased temporary data, unless it is an existing library file | Check source and cut visually |
| Exported poster/subtitle files | Only on explicit Save/Export or an admitted output policy | User-owned files | Portability outside CamelClip |

This feature uses captions already supplied by a provider, embedded in a local file, or explicitly imported by the user. CamelClip does not run speech recognition, translation, highlight detection or a heavyweight AI model.

## 2. Clarify what “preview” means

### 2.1 Poster

A poster is one image representing the recording. A provider thumbnail is an editorial image and MUST NOT be labeled as an exact frame from the video. A generated local frame is labeled separately and records its source timestamp.

The poster belongs in list rows, source details and the clip editor's initial empty-player state. It is not evidence of the selected interval or of video identity.

### 2.2 Timeline images

Timeline images are timestamped frames or provider storyboard tiles. They support hover/keyboard navigation and selecting approximate positions. They may be unavailable even when a poster exists.

A provider storyboard carries an explicit tile-to-source-time map and an estimated/exact distinction. An unsupported storyboard descriptor is ignored, not reverse-engineered by the frontend. Locally generated images record the requested and actual decoded frame timestamp.

### 2.3 Playable preview

Playable preview means local video/audio playback with a source-time clock and optional captions. It is available from an existing library file, a leased prepared source, or a compatible bounded proxy derived from that source.

A poster and transcript may be available before media acquisition. The UI MUST NOT make a disabled Play button look like playback is already available, and MUST NOT download the full recording merely because the editor opened.

## 3. Current architecture and integration gaps

| Current area | Inspected behavior | Required extension |
| --- | --- | --- |
| `core.ClipSource` | Identity, title, duration, chapters, streams and optional source digest | Associate companion catalogs by source identity; keep binary files and signed URLs outside this value |
| `core.MediaInfo` | Ordinary download metadata has no companion contracts | Add an optional separate discovery port; keep legacy Downloader consumers working |
| `application.ClipService` | Owns plans, source preparation, rendering and publication | Consume companion/source leases; never depend on a player widget |
| `ClipFiles.Cleanup` | Deletes a batch's prepared cache after all clips complete | Replace direct deletion with lease-aware source release before enabling previews |
| Current private sources | Owned per clip batch; no cross-batch reuse manager | Introduce a source-reference registry incrementally; do not pretend shared ownership already exists |
| Library assets | Single playable-file identity and existing file management | Add attachment relations; do not create fake videos/jobs for subtitle or image files |
| GUI transport | Loopback Host/Origin checks and an X-CamelClip-Token header | Add an authenticated media-read mechanism usable by native image/video elements |
| S3 uploads | Existing upload job represents one file | Add optional attachment delivery intents without changing the one-file uploader contract |
| SQLite schema | Version 6 | Allocate the next free migration version at implementation; 7 is only the current candidate |

Domain rules belong in `internal/core`; use cases, scheduling, retention and durable ownership belong in `internal/application`. Concrete yt-dlp, FFmpeg, HTTP, parser and filesystem work remain adapters. GUI/TUI/CLI and a future server consume the same services.

## 4. User journeys

### 4.1 Inspect a URL

1. The user explicitly inspects or adds a URL using existing authentication preferences.
2. Metadata discovery returns a finite source descriptor plus available poster and caption descriptors.
3. The UI shows title/duration immediately. Companion loading does not block this response on every binary download.
4. Visible rows request one appropriately sized poster through the application service.
5. Opening Transcript loads one selected caption track. No video is acquired for that operation.
6. A failure to load companions leaves manual downloading/clipping usable.

Pasting text without an explicit inspect/add action does not access browser cookies. Opening historical records offline does not silently trigger repeated provider authentication. Historical list rows are cache-only until the user explicitly refreshes the source or enables a documented background network policy. A missing cached poster shows a placeholder; rendering a row cannot reopen browser cookies to refresh an expired locator.

### 4.2 Choose a clip using text

1. Open the clip editor and inspect the source.
2. Open Transcript; review the selected language and Manual/Automatic/Translated/Unknown origin labels.
3. Search locally in the loaded text.
4. Click a cue to select its source position. If playback is available, seek there; otherwise select the position and offer Prepare preview.
5. Select a contiguous cue range and choose **Use as interval**.
6. Core constructs a proposed interval; the editor displays exact bounds and optional padding for review.
7. Admitting the plan still uses the existing integer range validation and immutable editorial intent.

Text availability never implies exact word-level timing or that the spoken sentence ends at the subtitle cue boundary.

### 4.3 Prepare playback

For a remote source without prepared media, Prepare preview presents the acquisition policy: a full source download may be necessary, its size may be unknown, and the prepared data can be reused for rendering compatible clips.

After acceptance, source preparation is a separate use case with its own identity and consumer references. It is not a fake empty clip batch or an ordinary library download. The later clip plan may reference that prepared source through an opaque, validated reference. No second download starts when the same compatible prepared source can serve the admitted batch.

The first playback implementation MUST use this local/full-source path. A direct provider embed, remote CDN player and partial-fetch player are outside the initial scope.

### 4.4 Keep companions with a saved video

If the user already loaded a poster and selected subtitle tracks, completing a whole-video download or publishing a clip can retain applicable companions in CamelClip's managed store. It does not fetch every unrequested track after completion.

A local library file may have no provider identity. It can use a generated poster, embedded text subtitle streams, or user-imported SRT/WebVTT files. CamelClip does not infer that a similarly named adjacent subtitle file belongs to it without confirmation/import.

## 5. Default settings and policy

Proposed defaults:

```json
{
  "media_companions": {
    "posters": "visible_on_demand",
    "subtitles": "on_demand",
    "preferred_languages": ["original", "interface"],
    "allow_automatic_captions": true,
    "allow_translated_captions": false,
    "keep_loaded_with_saved_media": true,
    "export_sidecars": false,
    "upload_companions": false,
    "playback_acquisition": "ask",
    "cache_budget_bytes": 536870912,
    "cache_idle_ttl_hours": 168
  }
}
```

`original` means the known source audio language, not the country of the provider. `interface` resolves the current application language. Unknown original language is skipped. No arbitrary English track is downloaded merely because it is the only one present; show the chooser instead.

Language preference outranks track origin: for each preferred language, prefer manual over automatic within that language. Match exact language/variant before a compatible primary-language fallback; never silently substitute hearing-description or translated variants. Translated tracks require explicit selection when the automatic translated-track policy is off. Unknown origin remains visibly unknown.

Loading captions for display may use a provider's existing automatic captions, with a clear label. It does not authorize generating a new transcript or translation. Changing application language does not replace an already selected/kept caption snapshot.

When a companion is loaded for an already saved asset, the same keep policy applies immediately; retention must not depend on catching a one-time video-completed event. Automatic caption retention chooses the currently selected track first, then the most recently explicitly selected compatible track, up to the two-track cap. Already pinned tracks are never removed to make room automatically. Additional tracks remain visibly cached until the user explicitly keeps them. A first poster may be retained automatically; replacing an existing kept poster or caption revision requires an explicit replacement action.

The cache budget above excludes playback/source video, which belongs to the source/staging resource policy. Permanently kept companions are not silently evicted under the cache budget; their disk use appears separately, with explicit management actions.

## 6. Source, credential and content identity

Every descriptor is scoped to workspace, canonical provider/item or local asset identity, authentication-context reference/revision, and metadata revision. Local generated artifacts additionally depend on the source file digest and selected timeline/stream mapping.

Poster identity includes provider image descriptor or generated-frame timestamp, target size and transform version. Subtitle identity includes provider track identity, language, origin, translation/variant information, raw digest, timeline-map revision and normalization version.

Never use title, row position, basename or an expiring signed URL as durable identity. Signed URLs are acquisition details: re-resolve when expired, verify unchanged source/track identity, and do not expose them in UI, logs, manifests or canonical digests.

Private material MUST NOT be shared across users or incompatible authentication contexts. If a provider/browser adapter cannot establish a stable private context reference, disable cross-session private reuse instead of assuming that `firefox` identifies the same account forever. Public-source reuse also requires explicit public classification, not merely an HTTP URL.

Provider captions and thumbnails are mutable. Refresh produces a new revision; it does not replace a kept track or alter an admitted clip plan. Kept snapshots remain usable offline after a provider removes or changes them, subject to the user's explicit local retention choices.

## 7. Storage layout and persistence

### 7.1 Managed store

Use a dedicated workspace-owned location, separate from batch source caches and the user's download directory:

```text
<workspace>/
  queue.sqlite3
  companions/
    blobs/<scope-hash>/<sha256-prefix>/<sha256>.<controlled-ext>
    staging/<attempt-id>/...
```

On macOS the workspace normally resides under `~/Library/Application Support/CamelClip`; on Linux it follows the existing configured workspace location. Honor `CAMELCLIP_CONFIG_DIR`. No state is written inside the application bundle.

Directories use owner-only permissions; sensitive/raw companion files are owner-only. Scope hashes avoid credential or source names in cache paths. Scope isolation is mandatory even if bytes happen to be identical. Deduplication occurs only inside compatible scopes.

Content-addressed blobs are immutable. Database rows associate them with source revisions and assets and record whether each reference is cached, kept, or actively leased. JSON manifests are export artifacts, not a second authoritative database.

### 7.2 What belongs in SQLite

| Record | Required fields |
| --- | --- |
| Companion source/catalog | Stable source/auth scope, metadata revision, discovery time, poster/track descriptors without signed URLs, availability and retry-after |
| Blob | ID, scope, digest, controlled format/MIME, byte size, private relative location, validation version, state |
| Reference | Blob/source/asset relation, role, language/track identity, cached/kept policy and creation time |
| Caption track snapshot | Raw and normalized blob IDs, origin/language, timing-map revision, normalization version and cue count |
| Acquisition/generation attempt | Entity/epoch, state, next retry, safe failure, temp ownership and publication intent |
| Lease | Resource/consumer identity, lifecycle kind, release/recovery evidence |
| Delivery/export intent | Selected files, destination snapshot, expected digests, per-file outcome and retry identity |

Index metadata and relations for source/asset/role/language queries. Do not store large image bytes or raw caption documents inside asset JSON or a batch row. Normalized cues may live in a paged JSON blob initially; an indexed cue table/FTS index requires a later explicit query and retention contract.

### 7.3 Crash-safe attachment publication

Acquire to a private attempt directory, validate, compute digests, persist a publication intent, then publish an immutable owned blob without overwrite. Commit the blob record/reference/caption snapshot together. Reconcile file-present/DB-missing and DB-present/file-missing cases on restart using recorded ownership and hashes.

Optional companions have their own completion state. A successfully published video remains done if a poster fetch fails. A user-requested **required** subtitle output is an explicit separate output policy, not the default best-effort attachment policy.

### 7.4 User-visible export

By default no `.jpg`, `.vtt` or `.srt` files clutter the download directory. **Export companions** writes selected copies next to the video or to a chosen destination:

```text
<video-base>.poster.jpg
<video-base>.<language>.<origin>.<track-short-id>.vtt
<video-base>.companions.json
```

Use controlled suffixes, collision-safe export identities and non-overwriting publication. Language tags and titles are not paths. Export manifests contain schema version, video/companion hashes, track origin and timing relationship, but no private paths, signed URLs or authentication selectors.

Managed blobs remain unchanged by export. Deleting an exported file does not invalidate the internal track. User-kept references are removed only through explicit companion/asset management; exported files are not recursively deleted by cache cleanup.

## 8. Retention, leases and cleanup

Default cache policy: 512 MiB for disposable posters, timeline images and caption files; seven days since last meaningful use. A list refresh does not refresh TTL for every historical row. Use least-recently-used eviction among unleased cache-only references.

Default negative-cache policy: unavailable poster/catalog/track results expire after six hours. Authentication failures are not recorded as “no subtitles”; rate limits follow provider Retry-After and bounded backoff. An explicit Refresh may recheck an absence result, subject to global throttling.

Active player streams, transcript views, poster generation, clip rendering and exports hold leases on their required resources. Closing/disconnecting a view releases its leases; process shutdown joins producers/readers before storage closes. Process IDs alone are not sufficient recovery evidence.

Rules:

- `clip-cache` cleanup MUST NOT delete a source currently leased for preview, even if every clip finished.
- A prepared source may be released after all media consumers finish while its kept poster/captions remain in the companion store.
- Unreferenced physical blobs are deleted only after reference reconciliation; bytes remain accounted until removal succeeds.
- Kept companions count toward a separate reported storage total, not an invisible unlimited cache. Offer filter-by-size/source and explicit unkeep/delete actions.
- Forgetting history does not delete library companions; forgetting an asset removes its relations only, preserving another asset's references.
- Deleting a video file does not silently delete captions the user separately pinned. The confirmation distinguishes video, associated managed companions and exported files.
- No automatic image/video proxy generation is started to fill a cache quota or “warm” all history.

## 9. Subtitle discovery and selection

### 9.1 Track descriptor

Each descriptor includes opaque track ID, provider language code and normalized display language, original label, manual/automatic/embedded/imported/unknown origin, translated true/false/unknown, optional hearing-description variant, available source formats and load capability.

Do not classify human/automatic/translated status from a language-code regex alone. Extractor metadata distinguishes provider manual and automatic catalogs where available; preserve uncertainty where richer classification is unavailable. Exclude chat/comment streams such as live-chat exports from caption selection.

Initial catalog limits: 200 visible track descriptors per source response, 50 per page. Discovery indicates truncation and offers an explicit bounded continuation/refinement. Never turn a discovery limit into “the other languages do not exist.”

### 9.2 Fetch one selected snapshot

Resolve the selected source/track again if its locator expired; acquire that track only, with shared network limits. Core verifies that provider/source/track/auth scope still matches. No `all languages` wildcard is passed as a default.

The yt-dlp adapter can use subtitle-only operation and format/language selection, but exact command templates must prove that no media, unrelated captions, posters or side effects are downloaded. Manual and automatic tracks of the same language remain distinct. If a CLI selector cannot distinguish them safely, use a descriptor-specific acquisition path or report the capability unavailable.

Initial source parsers support **WebVTT and SRT**. Text-based embedded subtitles may be converted through a tested adapter into this model. Bitmap subtitles/OCR, ASS styling/scripts/fonts and unsupported provider-specific formats are not silently interpreted. A tested converter can be added behind a capability/version gate later.

### 9.3 Normalization

Store both the original downloaded bytes and a versioned normalized cue model. The normalized model contains cue ID/order, integer source-time start/end milliseconds, plain text, optional speaker label, original timing precision and transformation diagnostics.

- Decode UTF-8, accepting a BOM. Other encodings require a declared, tested conversion or explicit user selection; never guess silently.
- Validate finite, ordered per-cue bounds and bounded lengths before indexing/rendering.
- Preserve overlaps between cues, duplicate spoken phrases and speaker order. WebVTT overlaps are valid.
- Normalize line endings and harmless whitespace while retaining meaningful line breaks. Never remove all repeated phrases globally.
- Strip executable/unsupported styling and render plain text using safe text nodes. Raw caption text is never inserted as HTML.
- WebVTT settings, regions, inline timestamps and provider rolling-caption updates need explicit handling. Preserve raw evidence; record what display normalization omitted.
- If a required timing map is ambiguous, permit a clearly labeled text-only view when safe, but disable seek/interval generation/export-as-synchronized-captions until resolved.

An optional display grouping layer may collapse overlapping incremental automatic-caption updates for readability. It is versioned, retains raw cue references and does not change the authoritative exported timing/content by heuristic sentence rewriting.

## 10. Caption timing and interaction with clipping

All normalized cue times refer to the same normalized **source presentation timeline** used by Core clipping. Provider zero is not assumed to equal media zero when timestamps, edit lists or partial sources differ. Store a declared timing-map identity and verification status: metadata-only, mapped, or unsupported.

A source-time cue is `[cue_start_ms, cue_end_ms)`. Selecting contiguous cue rows proposes the envelope of their bounds. The default added padding is zero; user-adjustable before/after padding is explicit and stored in the proposed interval. A noncontiguous selection is offered as multiple clips, never silently concatenated.

Core returns validation errors or a visible revised proposal for out-of-source bounds and sub-second intervals. The frontend does not silently clamp or pad to pass minimum-duration rules. A text click without a playable source selects a source timestamp; it does not imply a successful seek.

### 10.1 Captions accompanying a produced clip

Adding clip subtitle files is a separately admitted attachment-output option, default off. It does not burn captions into pixels or change the media render profile. For a requested clip `[A,B)` and a cue `[S,E)`, include the cue iff:

```text
S < B && E > A
output_start = max(S, A) - A
output_end   = min(E, B) - A
```

Require `output_start < output_end`; preserve deterministic source cue order and overlap. A cue crossing a cut is retained with shortened timing and marked boundary-truncated in provenance. Do not pretend its text was word-accurately cut. Word-level splitting requires a future contract.

The output clock relationship MUST be checked against the actual clip presentation timeline/evidence. If an offset is required by the produced file, apply and record that map; subtracting A blindly is insufficient for every container. If synchronization cannot be established, keep the track as source transcript only and block synchronized clip-caption export.

A user-admitted caption export pins the chosen track snapshot, source timing map, trim algorithm version and mode. Later caption refresh cannot alter that output. Caption failure does not rerender the verified video; retry targets the companion output. A required caption output yields a separate bundle `partial_failure` while preserving video `done`.

## 11. Poster and timeline-image acquisition

Prefer the smallest provider poster meeting the actual display size, with a proposed maximum download resolution of 1280×720 for ordinary posters. Do not fetch a 4K image for a 96-pixel row. Validate decoded dimensions, byte size, format and color conversion; normalize to a static JPEG/WebP/PNG variant through a bounded adapter.

Initial accepted image inputs are raster JPEG, PNG and WebP. SVG, HTML, unexpected animated formats and files whose MIME/decoded structure conflict are rejected. No image content is treated as executable markup. Unsupported provider poster formats produce a placeholder or an explicit local-generation fallback.

If no provider poster exists, a generic media icon is the immediate fallback. A generated local frame is optional only when local source data exists; it consumes a processing permit. Never download the full recording just to generate a poster.

Timeline generation starts only when the user opens/uses the timeline. Prefer an adapter-supported provider storyboard if available; otherwise generate bounded local thumbnails from a leased source. Initial local density: at most one image every ten seconds and at most 300 images per source, adaptively coarser for long recordings. Exact requested frame inspection is a separate on-demand operation; a coarse thumbnail never silently snaps an editorial cut.

## 12. Playback and proxy policy

1. Prefer direct playback of a supported local file, after capability detection.
2. Otherwise offer **Prepare playback copy**, explicitly showing extra work/storage.
3. Generate a bounded H.264/AAC MP4 proxy, maximum 720p, preserving aspect ratio without deliberate upscaling; audio-only preview can use a tested AAC-compatible profile.
4. Record source digest, proxy digest, selected streams, original source origin, proxy timeline map and transform version.
5. Display time through the source-time map. Proxy pixels/seek precision never replace Core's final cut verification.

A complete proxy may be necessary before this first implementation can promise seekability. Show Preparing/Unavailable honestly; do not assume a partially written MP4 is streamable. Progressive/segmented proxies are a later optimization with their own recovery contract.

The proxy uses a separate source/staging quota and shared processing resources, not the 512 MiB image/caption cache. Estimate size before admission when possible; unknown size is explicit. Starting a preview may wait behind existing processing, and canceling preview preparation must not cancel a shared source still required by render tasks.

Supported playback on native WKWebView/macOS and the Linux browser is determined by tests/capability probes, not by filename extension alone. An unsupported browser codec cannot make an otherwise valid source unclippable.

## 13. Core model and ports

Proposed values in `internal/core`:

- `CompanionSourceRef`: canonical identity, workspace/auth scope, metadata revision, optional asset/preparation reference.
- `CompanionCatalog`: revision/expiry, poster variants, caption descriptors, supported playback/storyboard capabilities and typed availability.
- `CaptionTrackSnapshot`: stable track/source identity, raw/normalized digests, language/origin, timing-map and parser versions.
- `CaptionCue`: stable index/ID, source bounds, plain text, raw cue reference and timing status.
- `PreviewDescriptor`: opaque resource ID, poster/storyboard/playback kind, source-time map, MIME, size, duration and capability/revision.
- `CompanionPolicy`: fetch, language, keep/export, limits and delivery settings resolved before execution where immutable intent is required.
- `AttachmentIntent`: chosen snapshot, output identity, best-effort/required policy and destination snapshots.

Proposed application services/ports:

```go
type CompanionService interface {
    Discover(context.Context, CompanionSourceRef) (CompanionCatalog, error)
    LoadPoster(context.Context, LoadPosterCommand) (PreviewDescriptor, error)
    LoadCaptions(context.Context, LoadCaptionCommand) (CaptionTrackSnapshot, error)
    Cues(context.Context, CaptionCueQuery) (CaptionCuePage, error)
    ProposeRanges(context.Context, CaptionRangeCommand) (CaptionRangeProposal, error)
    Keep(context.Context, KeepCompanionCommand) error
    Export(context.Context, ExportCompanionCommand) (AttachmentOperation, error)
    Close() error
}
```

Separate injected discovery/fetch, image decoder/transform, subtitle parser/normalizer, source-preparation, proxy generator, repository, immutable blob store, resource authority and lease ports. The application service selects policy and validates facts. The GUI cannot invoke yt-dlp/FFmpeg, choose arbitrary URLs to proxy, write caches, trim subtitle files or bypass language/auth rules.

Preview/read transport acquires an application lease and opens an approved opaque resource through a read port. Core does not depend on HTTP, browser cookies, DOM APIs or a desktop player. TUI/CLI can discover/fetch/export captions and propose intervals headlessly.

## 14. Commands, state and revisions

Acquisition states: `not_requested`, `queued`, `fetching`, `validating`, `ready`, `unavailable`, `waiting_authentication`, `waiting_rate_limit`, `failed`, `canceled`. Missing cached bytes transition to `not_requested`/recoverable missing, not a fabricated ready state. “Provider has none” and “request failed” are distinct outcomes.

All mutations carry entity and expected control/catalog revision where applicable. Long operations have attempt epochs; stale progress cannot replace a newer selected track or source. The result of an older search/fetch request is ignored by the view after its active source/track revision changes.

Concurrent loads of the same compatible descriptor coalesce into one acquisition with separate consumer references. Canceling one row/view releases that consumer; it does not cancel another user of the same resource. Offline cache hits never trigger authentication. Explicit Refresh changes metadata revision while pinned snapshots remain immutable.

Notification streams are bounded hints; authoritative snapshots and paged cues recover after reconnect. At most four visible progress updates/second per entity, checkpointing at bounded intervals; phase/control/terminal transitions persist immediately. No database write per video frame, keystroke or cue highlight.

## 15. GUI design

### 15.1 Downloads and library rows

Use a small fixed-aspect poster beside title; preserve a generic icon during loading/failure so row height never jumps. Request only visible/near-visible posters, not every historical row on each poll. Keep Reveal beside the actual saved video.

Optional metadata badges: CC available, selected language, Automatic. Missing poster/CC is neutral rather than a video-download error. Main row progress remains media download/processing progress; companion requests have independent state in details.

### 15.2 Video details

A player area shows poster, source/library distinction, Play or Prepare preview, and caption controls. A Transcript panel has language/origin chooser, local text search, timestamped virtualized rows, copy text, keep/export and refresh actions.

Loading and no-captions states are explicit. Authentication needed offers the existing browser selection workflow; it must not automatically open browser profiles or switch accounts. Turning captions Off hides the overlay without deleting a kept track.

### 15.3 Clip editor

Use three cooperating regions: player/poster, range list, and optional transcript. On narrow windows these stack; the transcript can collapse independently. The range list remains usable without any companion data.

- Source-time clock and an obvious distinction between source playback and playback of an output clip.
- Set Start/End from the current source position; keyboard actions and visible buttons.
- Transcript selection proposes bounds and highlights the corresponding range; final review shows the exact numbers.
- Find results show cue time, surrounding text and origin label; search never generates an AI answer.
- Follow playback highlights active cues. Manual scrolling suspends auto-follow until the user chooses Follow.
- Do not rebuild the player or focused transcript input on polling; preserve playhead, selection and scroll.
- Audio-only recordings use the same transcript; a video poster is optional.

A caption seek is navigation, not an edit. Existing ranges change only through explicit Set/Use interval actions. Poster clicks do not silently start remote acquisition.

### 15.4 Accessibility

Provide focusable controls, keyboard transcript navigation, language labels, screen-reader timestamps and visible focus in light/dark/system themes. State and automatic-caption origin cannot rely on color alone. Do not announce every highlighted cue through an assertive live region. Caption text size is configurable independently of the surrounding application font.

## 16. Authenticated media transport

Current header authentication is sufficient for fetch-based metadata, but native `<video>`, `<img>` and `<track>` requests do not provide an application-defined X-CamelClip-Token header directly. Do not “solve” this by exposing an anonymous file endpoint, putting the main session token in a URL, or loading arbitrary provider URLs in the frontend.

Recommended first implementation:

- Small posters and normalized cue pages use authenticated fetch. Poster responses become bounded Blob URLs, revoked when a view releases them. Captions are rendered from normalized safe cues; no raw remote track URL is embedded.
- Playback uses a media-session handshake: an existing authenticated POST creates a short-lived, memory-only server session and sets an HttpOnly, SameSite=Strict cookie scoped to a random media-session path and an unpredictable per-session cookie name.
- The endpoint remains exact-Host/port checked and loopback-only. Cookies themselves are not port scoped, so the random path/session namespace, exact host validation, lack of redirects and same-origin request checks are required, not optional details.
- Random paths are routing namespaces, not standalone security boundaries; every read still authenticates the cookie/session and authorizes the resource relationship.
- Media URLs contain opaque session/resource IDs, not credential values or private filesystem locations. Session IDs alone cannot authorize a request without the cookie.
- Validate Origin when present; reject cross-origin/cross-site fetch metadata where supported. Test actual WebKit/browser media requests before requiring headers that some supported clients omit. The host/cookie/path controls remain mandatory in that fallback.
- Cookie/session state is created only after the existing header-authenticated handshake, expires on shutdown and after bounded idle time, and is renewed explicitly by the frontend. Use Secure cookies for any future HTTPS transport; the existing HTTP loopback mechanism is not a public-server authentication design.

Playback supports GET/HEAD and one validated byte range per request, correct 206/416 behavior, bounded streaming, disconnect cancellation and active read leases. Responses are no-store/private, no-sniff, carry a controlled MIME type, and never redirect to a remote URL. Reject encoded path traversal, raw path parameters and directory listing.

An expiring playback session must allow a controlled re-handshake/reload while preserving source position. It must not drop authorization merely to keep a player running. A future multi-user server must additionally authorize workspace/principal/resource relations on every read and may use a different transport adapter without changing domain contracts.

## 17. Bounds, resource budgets and external input

Proposed initial hard limits:

| Item | Limit |
| --- | --- |
| Metadata document | 4 MiB, consistent with current bounded adapter behavior |
| Poster acquisition | 8 MiB compressed; 20 megapixels decoded |
| Caption raw file | 20 MiB per track |
| Normalized track | 100,000 cues; 32 MiB serialized |
| Cue text | 16 KiB UTF-8 per cue |
| Local search query | 512 Unicode characters |
| Cue/search page | Default 100, maximum 500 |
| Concurrent companion network fetches | At most two, also counted inside the existing global network/rate authority |
| Automatic poster requests per view | Visible/near-visible rows, at most 20 pending |
| Automatic keep after media save | One selected poster and up to two already loaded selected caption tracks |
| Explicitly kept caption tracks | Up to 32 per asset by default, with visible storage accounting |
| Active proxy generations | At most one, also counted in global processing limits |

Limits are Core/application policy with validated settings, not frontend constants. Reject or mark unsupported when exceeded; never silently truncate the tail of a transcript and present it as complete. Large transcript navigation/search is paged/streamed; do not send the entire cue set on every state poll.

Network/image/subtitle input is untrusted. Enforce timeouts, redirect limits, content/decoded-size caps, typed formats and bounded diagnostics. Discovery adapters must validate locators and auth scope; an artifact endpoint is not an arbitrary-URL fetch proxy. Local/import paths must be explicitly authorized assets/imports, not strings supplied by a remote client. Sanitize caption markup and image formats before display.

Image generation, source probes and proxy rendering share processing permits with ordinary downloads/clips. Never hold a network permit while indefinitely waiting for a processing permit. Do not create a separate helper pool that bypasses global budgets. User-triggered transcript/playback work may outrank speculative poster prefetch without starving media jobs.

## 18. Local imports, ownership and deletion

Import SRT/WebVTT into the managed store after the user selects the related asset and confirms the timeline relationship. Hash and copy it; do not edit the user's original file. An imported transcript with uncertain offset begins in text-only mode until an explicit offset/map is reviewed.

Embedded text captions are discovered by probing the local asset, then extracted only when selected. Reading subtitles must not modify or remux the input video. External adjacent files are offered as candidates only after explicit folder/import scope authorization.

Removing a subtitle from the overlay, removing a cached copy, unkeeping a track and deleting exported files are distinct UI operations. The default Remove companion action affects only the selected managed reference and explains whether another asset still uses the bytes. Existing video-delete and history-forget semantics remain recognizable.

## 19. S3 and portability

Default S3 behavior remains video/audio only. Companion delivery requires explicit selection in a batch/library upload policy. Do not infer this permission from global AutoUpload or from having viewed a transcript.

Use an attachment-delivery outbox with one stable identity per asset, snapshot/digest and destination revision. Each physical file is uploaded through the existing one-file uploader. A small optional manifest lists the logical bundle and hashes; no image/subtitle is registered as a fake playable asset just to reach the uploader.

Object keys use asset and attachment identities, not title/language alone. Caption/poster upload failure reports independent bundle progress and supports retry without re-uploading an already verified video. Cloud verification remains whatever the existing uploader actually proves; this feature does not silently upgrade size verification into checksum verification.

Companion exports contain enough information to preserve origin, source/clip timing and track choice. Retained raw caption bytes allow future normalization fixes; exported normalized VTT is for portability, while the internal raw snapshot remains immutable.

## 20. CLI/TUI and future interfaces

Proposed commands, not existing help:

```sh
camelclip companions inspect --url URL --json
camelclip companions captions --source SOURCE_ID --json
camelclip companions fetch-caption --source SOURCE_ID --track TRACK_ID
camelclip companions cues --snapshot SNAPSHOT_ID --query 'phrase' --cursor CURSOR
camelclip companions propose-range --snapshot SNAPSHOT_ID --from-cue 12 --to-cue 18
camelclip companions keep --asset ASSET_ID --snapshot SNAPSHOT_ID
camelclip companions export --asset ASSET_ID --out DIRECTORY
camelclip companions cache status
camelclip companions cache prune --dry-run
```

TUI exposes a transcript/details panel, language/origin selection, local search and interval proposal. It may open a local video in an external player through the existing desktop port, but cannot depend on that player for Core range validation. Headless CLI discovery/fetch/export/proposal works without a desktop, GUI media cookie or native browser.

Future server clients receive the same descriptors, paged text and commands after server authorization. Distributed preview workers, multi-user caches and public sharing are not included in this local feature.

## 21. Acceptance matrix

| ID | Scenario | Required result |
| --- | --- | --- |
| COMP-01 | Inspect a URL without opening transcript/player | Metadata only; no media or all-language fetch |
| COMP-02 | Twenty visible rows amid ten thousand records | Bounded poster requests; unrelated history untouched |
| COMP-03 | Poster/CC missing, source video valid | Download and manual clipping remain usable |
| COMP-04 | Manual and automatic captions share a language | Distinct tracks and deterministic labeled selection |
| COMP-05 | Translated/unknown-origin catalog entries | No silent human/original classification |
| COMP-06 | Signed locator expires | Re-resolve same scope/identity or typed failure |
| COMP-07 | Authentication account/context changes | No private cached material leaked across scopes |
| COMP-08 | WebVTT overlaps, rolling updates and repeated speech | Raw evidence retained; no global text loss |
| COMP-09 | Malformed/oversized captions, markup and image bombs | Bounded rejection; no HTML/script execution |
| COMP-10 | Source offset/VFR/proxy clock differs from provider zero | Verified source-time map or text-only/unsupported state |
| COMP-11 | Cue selection crosses duration/minimum constraints | Core returns reviewed correction/error; no hidden clamp |
| COMP-12 | Caption cue crosses clip start/end | Deterministic intersection/rebasing; truncated-text flag |
| COMP-13 | Source cleanup races with active playback | Source retained until the last valid lease releases |
| COMP-14 | One of two preview/transcript consumers closes | Other consumer remains usable; acquisition not duplicated |
| COMP-15 | Cache eviction while a kept track is offline | Kept track survives; only unleased disposable data evicted |
| COMP-16 | Crash between blob publication and relation commit | Reconcile one attachment or safe cleanup, no guessed ready state |
| COMP-17 | Raw track refresh after clip admission | Existing pinned snapshot/output unchanged |
| COMP-18 | Anonymous/cross-origin/traversal/invalid Range read | Rejected; no arbitrary local/remote file access |
| COMP-19 | WKWebView and Linux browser playback auth/expiry | Authenticated seeking works; renewal preserves position |
| COMP-20 | Companion S3 disabled, video AutoUpload enabled | No poster/caption objects uploaded |
| COMP-21 | Subtitle export/upload fails after media completion | Video remains done; independent retry/bundle outcome |
| COMP-22 | Library import or history removal | No modification of original media/caption files; shared references preserved |
| COMP-23 | Shutdown during proxy/fetch/player read | Children/readers joined before storage close; restart consistent |
| COMP-24 | GUI, TUI and CLI use the same cue selection | Same proposed canonical intervals and validation |

Tests use local media, saved provider-metadata fixtures, local HTTP fixtures and synthetic captions. They do not use real browser cookies, private video accounts or cloud credentials. Include Arabic/RTL, CJK, emoji, BOM/line endings, language variants, audio-only sources and unavailable codecs. Native player tests are recorded separately from headless Core tests.

## 22. Implementation order and gates

1. **Core catalog/policy and storage:** source/track identity, immutable blob references, bounded parsers, scopes, retention and migrations. Gate: headless contract tests and nonempty schema-6 migration/reopen.
2. **Poster and caption discovery/acquisition:** selected-language fetch, availability/negative cache and image validation. Gate: no hidden media download, rate/auth isolation, malformed-input tests.
3. **Transcript and interval proposals:** paged cues/search, origin labels, GUI/TUI/CLI use of Core bounds. Gate: equivalent headless/frontend proposals and preserved raw timing/text.
4. **Shared source leases:** replace direct successful-batch source deletion and allow a preview preparation to attach to a compatible clip plan. Gate: cleanup/cancel/restart races and one source acquisition.
5. **Authenticated local playback/proxies:** media handshake, Range reads and source-time mapping. Gate: native WebKit/Linux browser authorization and offset fixtures; no anonymous endpoint.
6. **Optional kept/exported clip captions and S3 companions:** snapshot pinning, cue intersection/rebasing, per-file intents and manifest. Gate: boundary fixtures, no media rerender on companion failure, explicit delivery policy.
7. **Optional timeline storyboards:** adapter-supported provider tiles or bounded local generation. Gate: time-map accuracy, resource limits and no unrequested video acquisition.

An initial useful release may stop after step 3, explicitly providing posters/transcripts without playable preview. Playback cannot ship before lease-aware cleanup and authenticated reads. Caption slicing cannot be labeled synchronized until its output-time-map tests pass. Update the operational docs with the exact implemented subset; do not present proposed commands as existing ones.

## 23. Settled decisions

- Keep this functionality in independent Core/application services, not in JavaScript or a specific frontend.
- Distinguish posters, timeline images and playable media in contracts and UI.
- Discover track availability cheaply with existing metadata work; acquire only selected companions.
- Preserve fetched, selected companions with saved assets by policy, without fetching every unrequested language.
- Keep disposable cache separate from durable user-kept references; no implicit exported sidecar clutter.
- Use source-time integer bounds, immutable caption snapshots and explicit timeline mapping.
- Manual/automatic/translated/unknown labels and unavailable states are first-class data.
- No full-video download on hover, no heavy local AI, no implicit subtitle burning or cloud upload.
- Player readers must hold source leases; batch completion alone is not permission to delete an active preview source.
- Media authorization must work with actual native-player requests while preserving loopback/session protections.

## 24. Primary references and evidence boundaries

- [yt-dlp subtitle options](https://github.com/yt-dlp/yt-dlp#subtitle-options) and [thumbnail options](https://github.com/yt-dlp/yt-dlp#thumbnail-options): upstream supports selecting available subtitles/automatic captions and writing thumbnails; its skip-download mode can acquire auxiliary files without downloading video. Exact provider/track behavior requires fixture tests against supported versions.
- [WHATWG HTML media elements](https://html.spec.whatwg.org/multipage/media.html): browser-managed media fetching, resource requests and playback behavior; the transport adapter must be tested with actual supported players.
- [HTTP cookie specification](https://httpwg.org/specs/rfc6265.html): cookies do not provide isolation by port; the proposed loopback media-session namespace must account for this.
- [W3C WebVTT](https://www.w3.org/TR/webvtt1/): timed text syntax, overlapping cues, cue payload and timing semantics. CamelClip normalization and plain-text rendering are deliberately narrower than arbitrary authored styling.
- [Clip specification](clip-by-link-specification.md), [implemented clip scope](../clips.md), [architecture](../architecture.md), [S3](../s3.md) and [GUI](../gui.md).

Upstream references were checked on 2026-10-23. All defaults, quotas, service shapes, storage choices, security handshake, UI behavior and implementation gates above are CamelClip proposals, not claims that current code or upstream tools already implement them.

## Implementation status — 2026-10-23

The first increment implements stages 1–3: source discovery, bounded poster and
caption acquisition, immutable snapshots, offline paged transcript search, GUI/TUI/CLI
access, explicit SRT/WebVTT import and WebVTT export, and reviewed cue-to-clip
range proposals. SQLite schema 7 stores catalogs, blobs, references and snapshots.
The current implementation deliberately serializes companion operations, uses fixed
cache policy and isolates fresh discovery scopes. Budget cleanup occurs before
fetches or explicitly, and may exceed its target by one fetch. Playback, source
leases/proxies, storyboard generation, clip caption projection, S3 companion
delivery, cache orphan reconciliation and configurable policies remain planned.
This status qualifies the future requirements above; see [current usage](../companions.md).
