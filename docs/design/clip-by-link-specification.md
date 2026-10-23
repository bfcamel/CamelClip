# Clip by Link — Product and Engineering Specification

- **Status:** Precise local/full-source increment implemented; remaining v1 scope proposed. See section 37.
- **Specification version:** 1.2 (implementation contracts and increment status).
- **Date:** 2026-10-23.
- **Baseline:** CamelClip main `3c40266de4760a632d821e067a6cf45b2f2f5089`.
- **Audience:** Core, persistence, downloader/media adapters, GUI/TUI/CLI, and QA contributors.
- **Scope of this document:** Define the feature and its integration. This document does not authorize a release, installer rebuild, application update, or implementation.
- **Normative terms:** MUST/MUST NOT are requirements; SHOULD is the default unless a documented reason justifies an exception; MAY is optional.

## Reading and implementation order

Sections 1–24 define product behavior and the initial architecture. Sections 25–36 make that design executable: dependency boundaries, public commands, transaction semantics, concurrency, media evidence, adapter requirements, acceptance scenarios and delivery gates. Where an earlier sketch leaves a choice open, the explicit decisions in sections 25–36 take precedence. All code snippets are proposed contracts, not declarations already present in the repository.

“Independent Core” means that business decisions and durable behavior work without any frontend. In the current repository this comprises domain rules in `internal/core` and use-case orchestration in `internal/application`. It does **not** mean placing subprocess management, SQL, operating-system integration or every worker inside the `core` package.

## 1. Product definition

Clip by Link creates independently playable media files from user-selected intervals of one finite source video or audio recording. A user can paste a URL, enter several ranges or select chapters, name the outputs, and obtain verified clips with source provenance. An existing local library asset can be used instead of a URL.

The feature is not a general timeline editor. Each range produces one asset; overlapping ranges remain independent. It does not concatenate ranges into a montage, crop the image, add transitions, infer highlights, transcribe speech, or record a live stream.

### 1.1 User outcome

Given a two-hour recording, a user requests:

| Output title | Source interval | Result |
| --- | --- | --- |
| Introduction | 00:02:10.000–00:05:00.000 | One 2 min 50 sec clip |
| Demonstration | 00:31:20.000–00:37:45.000 | One 6 min 25 sec clip |
| Questions | Selected source chapter | One clip with resolved chapter bounds |

The source is acquired once per compatible batch, not once per interval. Each output appears separately in the library, supports the existing per-file Reveal action, and can have independent S3 delivery.

### 1.2 Success measures

- A user can create several clips without downloading a new full source for each clip.
- The interface distinguishes requested bounds, effective bounds, processing mode, source acquisition, and output rendering.
- An interrupted batch can resume without replacing existing files or redoing completed clips.
- No unverified output is reported as complete or admitted to automatic upload.
- Existing whole-video downloads and playlist monitoring retain their behavior.

## 2. Scope and delivery boundaries

### 2.1 Complete v1 scope

1. One source URL or one existing library asset per batch.
2. Manual ranges and source chapters when available.
3. Multiple independently named outputs in video or audio mode.
4. Precise mode and explicit fast-copy mode, with different accuracy contracts.
5. Shared source preparation, bounded resources, cancellation, retries, and restart recovery.
6. GUI, TUI, and CLI access to the same application service.
7. Local provenance and optional explicit export of a metadata manifest.
8. Optional selected S3 destinations using the existing encrypted vault and upload queue.
9. Source preview when a suitable local source is available; lack of preview does not prevent manual clipping.

### 2.2 Not required for the first increment

The first shippable increment MAY provide precise mode, manual ranges, local outputs, and all three frontends before fast-copy, chapters, preview, and S3 arrive. It MUST label unavailable capabilities rather than pretending the complete v1 scope has shipped.

### 2.3 Explicit exclusions

- Montage/concatenation, transitions, image cropping, aspect-ratio conversion, overlays, audio mixing, effects, or arbitrary FFmpeg arguments.
- AI highlight detection, speech recognition, transcript search, scene detection, or automatic editorial decisions.
- Playlist-wide clipping templates and automatic clipping of future subscription entries.
- DRM bypass, live recording, and infinitely growing streams.
- Remote object deletion, archive retention policies, or a new server/daemon.
- Guaranteed bandwidth savings or a hard completion-time promise for remote URLs.
- HDR preservation, multitrack selection, surround preservation, and subtitle slicing in v1. These need separate explicit contracts.

## 3. Existing architecture and integration constraints

| Existing area | Current behavior | Required response |
| --- | --- | --- |
| [Core downloader](../../internal/core/core.go) | `Download` returns one `Result.Path`; optional Published refers to the same file | Keep this interface intact. Do not return many clips through one Result or publish several paths through one legacy observer |
| [Download adapter boundary](../../internal/application/download_contract.go) | Serializes callbacks and validates monotonic stages and one result | Reuse the principles; provide a separate typed clip contract with per-task identity |
| [Verified pipeline](../../internal/downloader/ytdlp/verified.go) | Private staging, source download, optional merge/conversion, full decode, publication without overwrite | Extract reusable preparation, verification and publication primitives; do not invoke a normal library download for a hidden temporary source |
| [Video conversion](../../internal/downloader/ytdlp/video_profile.go) | Can copy already matching codecs | Precise clipping must explicitly force encoding even if codec/container already match |
| [Verification](../../internal/downloader/ytdlp/verified.go) | Whole-media duration tolerance can be as broad as max(3 seconds, 5%) | This is insufficient for clip boundary acceptance; introduce clip-specific timeline verification |
| [Resources](../../internal/application/resources.go) | Queue owns bounded download/processing resources | Both ordinary downloads and clipping must use one shared resource authority |
| [Publication/catalog](../../internal/storage/sqlite/catalog.go) | Publication reconciles assets and upload admission transactionally | Extend durable publication reconciliation to clip task identities without inventing fake download job IDs |
| [Playlist materializations](../../internal/storage/sqlite/playlists.go) | Durable whole-video membership and materialization identity | Clipping must not mark a subscription entry downloaded or replace its whole-video asset |
| [Workspace lifecycle](../../internal/application/workspace.go) | Services stop before the queue closes its repository | Clip workers, previews and source leases must stop/join before storage and resource shutdown |
| SQLite schema | Current maximum schema is 5 | Use the next free schema version at implementation time; 6 is the baseline proposal, not a permanent reservation |

Adding range fields to the legacy Request and silently teaching Download to emit multiple files is prohibited. Existing Request.Duration remains source/request metadata; it must not be repurposed as a clip endpoint.

## 4. User scenarios

### 4.1 Paste a URL and enter ranges

1. Open **Create clips** from Add links or the downloads page header.
2. Enter one source URL and explicitly request inspection.
3. Review source identity, duration, available chapters, and acquisition estimate.
4. Add intervals with Start, End, and optional Title.
5. Choose output profile and precise/fast mode.
6. Review the plan, including possible full-source acquisition and destination directory.
7. Create the batch. Each output receives an independent task.
8. Observe acquisition once, then per-clip processing/verification.
9. Reveal/play completed clips or request S3 delivery.

Typing or pasting alone MUST NOT read browser cookies, download media, or begin processing.

### 4.2 Clip an existing library file

A **Create clips** action beside a local video opens the same editor with an asset reference. No network or browser authentication is required. The source file is never edited or moved. Its identity is rechecked before processing and after relevant reads.

If the asset is missing or cloud-only in a future archive feature, the operation is blocked with an explicit restore/source selection action. It must not silently fetch a URL that might now represent different content.

### 4.3 Select chapters

The user selects chapters from inspected metadata. The plan expands them into explicit immutable ranges and records the chapter title/index and metadata revision. Chapters are labels and timestamps, not regular expressions or executable extractor instructions.

### 4.4 Retry a partially completed batch

Completed clips remain complete. Retrying targets only selected failed/canceled tasks, reuses an intact prepared source where possible, and never renames or overwrites a completed sibling.

### 4.5 Cancel

Cancel one clip without canceling siblings. Cancel a batch to stop uncompleted work. Already published clips and verified uploads remain available. Cancellation is a control request, not a request to delete outputs.

## 5. Time model and validation

### 5.1 Canonical representation

All public bounds are signed 64-bit integer milliseconds, validated as nonnegative. Source intervals are **half-open** `[start_ms, end_ms)` relative to the beginning of the source's normalized presentation timeline.

The application MUST NOT use binary floating-point seconds as durable range identity. Adapters may convert to rational time bases at the FFmpeg boundary. JSON inputs must be integers and remain within JavaScript's exact integer range; the v1 duration limits below are much smaller.

### 5.2 Accepted user input

- Seconds: `95`, `95.250`.
- Minutes and seconds: `01:35`, `01:35.250`.
- Hours, minutes and seconds: `02:01:35.250`.
- CLI range: `00:12:30.000..00:15:10.000`.
- GUI/TUI use separate Start and End fields to avoid separator ambiguity.

At most three fractional digits are accepted; excess precision is rejected, not rounded silently. In colon notation, seconds and all non-leading minute fields must be 00–59. Negative, exponent, NaN, infinity, locale commas and relative-to-end inputs are rejected in v1. Whitespace around a field is trimmed.

Examples: `90` means 90 seconds; `01:30` means the same instant. `1:90` is invalid.

### 5.3 Bounds and batch limits

Proposed v1 admission defaults:

| Constraint | Value |
| --- | --- |
| Sources per batch | Exactly 1 |
| Intervals per batch | 1–100 |
| Minimum interval duration | 1,000 ms |
| Maximum source duration | 7 days |
| Maximum sum of requested output durations | 24 hours, including overlaps |
| Maximum clip title | 200 Unicode scalar values and 800 UTF-8 bytes |
| Maximum plan JSON body | 1 MiB |
| Maximum admitted, unfinished clip tasks per workspace | 1,000 |
| Maximum source preparations admitted at once | 100; actual workers use shared limits |
| Plan lifetime before admission | 24 hours |

A range must satisfy `0 <= start < end <= authoritative_source_duration`. End is explicit; there is no unbounded `inf` endpoint. Exact duplicate canonical intervals within one batch are rejected, even if titles differ. Overlaps and out-of-order intervals are accepted and preserved in user order; the preview explains overlap creates separate repeated content.

Metadata duration is provisional until a local prepared source is probed. If the authoritative timeline makes any range invalid, block the entire batch before rendering and require a revised plan; do not silently clamp, shorten or shift the request. All ranges must pass authoritative validation before any output renders.

Unknown-duration media may be inspected, but admission stays blocked until a finite duration is established. A user may explicitly authorize local preparation to resolve it; this is not part of an inspection-only request.

### 5.4 URL time hints

For supported extractors, a URL time hint may prefill Start, but it must be shown in the editor and confirmed. Explicit range fields take precedence. An absent end hint does not imply the source's end without a visible user choice. Malformed hints produce a nonfatal explanation and an empty range, not an arbitrary cut.

A video URL with playlist context is resolved as one explicitly identified video. A pure playlist, channel, multi-video result, live stream, upcoming premiere or DRM-protected source is not admitted as a finite clip source.

## 6. Accuracy contracts

### 6.1 Precise mode — default

- Re-encode selected video and audio streams; do not take the existing codec-match copy shortcut.
- Decode sufficient pre-roll to reach the requested source presentation timestamp.
- Select the first presented video frame at or after Start and exclude frames whose presentation timestamps are at or after End.
- Trim audio on its decoded sample timeline, taking source offset and codec delay into account.
- Normalize output timestamps while preserving the selected streams' relative synchronization. Independently resetting each stream in a way that changes A/V offset is prohibited.
- Persist the measured effective boundaries and tolerance evidence.

“Precise” means precision at the source's frame/sample resolution, not an arbitrary sub-frame visual cut. Display this distinction near the mode selector.

Normative acceptance thresholds:

| Measurement | Acceptance |
| --- | --- |
| First selected video frame | Matches the source timeline's first presented frame at or after Start |
| Last selected frame | Has source PTS below End; its display duration may extend by one local frame interval |
| Audio boundary error after encoder-delay accounting | No more than max(20 ms, one encoded audio frame) |
| A/V offset introduced by clipping | No more than max(50 ms, one local video frame interval plus one audio frame) |
| Container duration difference from selected decoded media | No more than max(100 ms, two local video frame intervals, two encoded audio frames) |

For variable-frame-rate material, use actual neighboring frame timestamps, not an average FPS. Sources whose timing cannot be mapped unambiguously fail with `clip_timeline_unsupported`; they are not published under a precise claim.

The renderer must retain bounded timeline evidence: original stream time bases and offsets, the first/last selected source presentation timestamps, neighboring frame intervals at the cuts, and the encoder-delay accounting method. Do not infer source boundaries from the output container duration alone. Evidence may be collected during rendering/verification without persisting a per-frame log for the entire source. A range containing no valid selected video frame or no required audio samples fails explicitly instead of producing an empty or padded success.

These thresholds are implementation requirements to prove with fixtures. They are not claims about the existing verifier.

### 6.2 Fast-copy mode — explicit approximation

Fast-copy copies compressed streams without re-encoding. It is available only if selected codecs/streams already satisfy the requested output profile and the adapter can establish independently decodable boundaries.

1. Prepare a local source and discover suitable boundaries.
2. Propose the nearest usable preceding video random-access point and an ending boundary covering the requested interval, capped by source end.
3. Show requested and proposed intervals separately, including added content.
4. Require acceptance of the effective-range revision before rendering.
5. Decode-verify every output. A keyframe flag by itself is not sufficient proof of independent playback.

Default maximum expansion is 2,000 ms at either boundary. If no suitable boundary exists within that bound, fast-copy is unavailable for that interval. Offer precise mode; do not switch automatically. A larger expansion is outside v1.

Audio-only fast-copy uses encoded packet boundaries and the same explicit effective-range approval. If the selected format requires conversion, precise mode is required.

Actual fast bounds are often unknown before source preparation. The editor must distinguish the initial estimate from the later approval step. Approval applies to all unresolved fast intervals in that batch revision; no fast output publishes before its effective bounds are accepted.

### 6.3 Mode changes and defaults

- Never silently downgrade precise to fast-copy.
- Never silently upgrade fast-copy to a costly transcode.
- Codec/container incompatibility is a validation error with a suggested valid profile.
- Default video output is the existing H.264/MP4 profile; default audio output follows the explicit audio preference.
- Video output requires the same selected video/audio stream expectations as the existing supported video pipeline. Silent video and advanced stream selection require a later deliberate extension.
- HDR sources are blocked for v1 until tone-mapping/preservation has a tested contract; they must not silently become incorrectly colored SDR clips.

## 7. Source inspection, acquisition and reuse

### 7.1 Source identity

Inspection returns extractor/provider identity, canonical item ID, sanitized source URL, title, finite duration when known, selected format information, available chapters and capability warnings. Do not use the mutable display title or an expiring CDN URL as identity.

Persist the inspected identity and resolved profile. Immediately before preparation, revalidate identity. A changed provider ID, incompatible timeline or source revision blocks the affected plan. A refreshed signed URL for the same source is not itself a content change.

For a local asset, capture asset ID, canonical absolute path, regular-file identity, size, modified time and a content digest before source reuse. Stat alone is not an adequate persistent content fingerprint.

### 7.2 Acquisition policy

A plan declares one of:

- `local_only`: use a verified local asset/cache entry; never fetch remote media.
- `full_source`: acquire the selected finite source once.
- `prefer_sections`: attempt an explicitly supported partial strategy; full-source fallback is allowed only if the admitted plan explicitly permits it.

The complete first implementation should use local/full-source acquisition as its correctness baseline. Partial fetching is a later optimization behind capability checks and parity tests.

The initial plan must state that saving a small clip may still require downloading the whole selected source. Unknown size is displayed as unknown. If full-source fallback was not authorized, transition the source to `waiting_approval` with reason `source_policy_approval_required` instead of starting it.

### 7.3 Shared preparation

- A batch has one durable SourcePreparation; 100 intervals do not start 100 extractor downloads.
- Compatible batches may share a preparation only within the same workspace and authentication context, selected source identity/profile and acquisition strategy.
- Do not share private-source caches across credential contexts or users. Authentication selectors themselves are never embedded in public cache paths.
- Use a prepared-source digest as final content evidence; URL and metadata alone do not prove byte identity.
- Prepare selected source streams without final clip-profile transcoding. Separate audio/video downloads may require a private merge or a stream descriptor consumed by the renderer.
- Do not use `Downloader.Download` to publish a hidden full source into the user's library or trigger global AutoUpload.

### 7.4 Private cache and leases

Store prepared data in a workspace-owned private directory with 0700 directories and 0600 sensitive manifests. Do not place it next to unrelated user files without an explicit configured staging root.

Every source has durable references and in-process/file locks. Reading/rendering/preview pins a lease. Canceling one task releases only its own reference; an unreferenced preparation may be canceled. A source required by another task cannot be cleaned up.

Proposed cache policy: retain unreferenced prepared sources for 24 hours, with a configurable 20 GiB soft cache budget. The workspace staging/free-space policy always takes precedence. Never evict leased data to meet the budget. If protected data exceeds capacity, stop admission and explain the pressure. Cached bytes and pending cleanup remain counted until actually removed.

On restart, reconcile durable references with tasks and locks; do not trust stale process IDs. Existing child-process/staging locks remain necessary so an orphan FFmpeg process cannot race cleanup.

## 8. Chapters

Add optional chapter metadata through a dedicated source descriptor; do not require existing legacy MediaInfo consumers to understand it.

Each chapter contains source-order index, title, start/end milliseconds and metadata revision. Duplicate titles are allowed; indexes are the selection key. Invalid individual chapters are listed as unavailable with a reason. Valid chapter ranges must meet the same interval constraints.

Missing chapter end may be derived from the next valid chapter start or the known finite source end; record that derivation. Overlapping, gapped and out-of-order chapter metadata is not silently rewritten into an invented editorial structure.

Selection expands to immutable explicit intervals at plan admission. Later extractor metadata changes do not alter an admitted request. A range edited manually is marked as a user range with optional chapter provenance, not as an unchanged chapter selection.

## 9. Proposed domain model and ports

The following names define responsibilities, not final source-compatible Go declarations.

```go
// Milliseconds are validated before arithmetic or adapter conversion.
type ClipRange struct {
    StartMS int64
    EndMS   int64
}

type ClipSegmentRequest struct {
    ClientID     string
    Title        string
    Range        ClipRange
    ChapterIndex *int
}

type ClipBatchRequest struct {
    Source         ClipSourceRef // exactly one URL or local asset ID
    Segments       []ClipSegmentRequest
    Accuracy       ClipAccuracy // precise | fast_copy
    Acquisition    ClipAcquisitionPolicy
    Output         ClipOutputProfile
    OutputDir      string
    S3ProfileIDs   []string
    ExportManifest bool
}

type ClipSourceProvider interface {
    Resolve(context.Context, ClipResolveRequest) (ClipSourceDescriptor, error)
    Prepare(context.Context, ClipPreparationRequest, ClipSourceObserver) (PreparedClipSource, error)
}

type ClipRenderer interface {
    PlanBounds(context.Context, PreparedClipSource, ClipRenderRequest) (ClipEffectiveRange, error)
    Render(context.Context, PreparedClipSource, ClipRenderRequest, ClipObserver) (ClipStagedResult, error)
}
```

### 9.1 Required supporting records

- **ClipPlan:** ID, revision, expiration, canonical source descriptor, normalized request, estimates/warnings, digest of reviewable intent.
- **ClipBatch:** ID, immutable admitted plan revision, creation/idempotency identity, ordered task IDs, control state and aggregate counts.
- **ClipTask:** ID, batch/segment identity, requested/effective range revisions, immutable output request, attempts, error/wait reason, staged/publication identity, asset ID and metrics.
- **PreparedClipSource:** ID, content digest, selected streams, authoritative timeline/duration, capability evidence, private location reference and lease identity.
- **ClipProvenance:** source identity, original/source title, requested/effective ranges, accuracy mode, source/output digests, profile/pipeline version, chapter origin, creation time.

Prepared source references exposed through the API are opaque IDs, not private filesystem locations. Credentials and credential material remain in existing security/cloud services.

### 9.2 Application component boundaries

- **ClipPlanner:** normalization, validation, immutable plans, resource estimates and approval requirements. No subprocesses or filesystem deletion.
- **ClipSourceCoordinator:** acquisition deduplication, cache references, source recovery and preparation lifecycle.
- **ClipExecutor:** one task's bounded render/verify progression and cancellation.
- **ClipPublisher:** publication identity, filesystem intent journal and catalog reconciliation.
- **ClipService:** public commands, lifecycle coordination and paginated read models; not a second media engine.
- **ClipRepository:** transactions, CAS revisions, stable indexes, durable task/source/publication records.

Bootstrap injects adapters and repositories. UI packages do not import downloader, FFmpeg or SQLite implementations. Core types do not import application/UI/storage. If reusable FFmpeg helpers move to an adapter package such as `internal/media/ffmpeg`, update architecture dependency tests deliberately; do not weaken all dependency rules to accommodate the new package.

### 9.3 Resource integration

Extract/inject the existing resource authority so source preparations and ordinary downloads share network limits; rendering and full-decode verification share processing limits. A metadata limit applies to source inspection as well. Do not create an independent default worker pool that doubles configured concurrency.

No task may hold a network permit while indefinitely waiting for a processing permit or vice versa. Source preparation and clip rendering release/acquire stage leases explicitly. Existing rate budgets must cover acquisition; adding an external downloader must not bypass rate/cancellation controls.

## 10. State machines and commands

### 10.1 SourcePreparation states

`queued -> resolving -> waiting_resources -> downloading -> preparing -> ready`

Possible exits: `waiting_approval`, `waiting_authentication`, `blocked_disk`, `failed`, `canceling`, `canceled`. Local reuse may move from resolving directly to ready after validation. Every exit preserves a typed reason.

### 10.2 ClipTask states

| State | Meaning |
| --- | --- |
| `queued` | Admitted and persisted |
| `waiting_source` | Shared source not ready |
| `waiting_range_approval` | Fast effective bounds need acceptance |
| `waiting_processing` | Ready but no processing lease |
| `rendering` | Producing a private candidate |
| `waiting_verification` | Candidate exists; waiting for a verification lease |
| `verifying` | Full decode/profile/timeline checks |
| `publishing` | Durable publication intent exists |
| `done` | Verified local asset committed/reconciled |
| `paused` | No new stage may start |
| `canceling` / `canceled` | Cooperative cancellation is pending/completed |
| `failed` | Attempt failed; error and retry classification persisted |

`blocked_disk` and `waiting_authentication` may be represented as waiting reason codes rather than additional task states. Do not multiply states for every error condition.

### 10.3 Batch aggregate

Expose control status separately from progress: `queued`, `running`, `waiting_approval`, `paused`, `canceling`, `canceled`, `completed`, `partial_failure`, `failed`. Aggregate counts are authoritative: total, done, failed, canceled, active and waiting.

A settled batch with any failure and some successful clips is `partial_failure`; all failed is `failed`. A user-canceled batch with any canceled unfinished work remains `canceled` even if some clips completed. If publication reconciliation establishes that every task had already completed, the aggregate is `completed` and cancel reports already completed. `completed` requires every requested task done. S3 completion is a separate aggregate and never rewrites local completion.

### 10.4 Public commands

- Resolve/inspect source; create/update a plan before admission.
- Admit plan with expected revision and idempotency key.
- Accept effective bounds with an expected bounds revision/digest.
- List/get batches/tasks and read provenance.
- Pause/resume/cancel a batch or selected tasks.
- Retry selected failed/canceled tasks; completed tasks are immutable.
- Reveal/play an existing published asset.
- Export a sanitized provenance manifest.
- Forget terminal task history without deleting assets or dedup/publication evidence.

Editing bounds, source, profile or destination directory after admission creates a new plan/batch. Cosmetic display-title edits may be supported separately, but do not rename an already published file implicitly.

## 11. Progress and observability

- Display shared acquisition once per batch, not copied into every child as independent network work.
- Source progress includes downloaded bytes, actual/estimated total, speed and ETA when known.
- Each task reports rendering and verification progress against its effective interval.
- Source time positions and output duration are separate fields. A zero-based output clock is not a source timestamp.
- Unknown totals use the existing indeterminate indicator; never synthesize a fixed byte total.
- Batch summary displays `3/8 clips ready`; an optional weighted percentage must be labeled estimated and disclose its weighting.
- Local 100% is reached only after publication/catalog reconciliation. S3 progress is independent.
- Progress events are coalesced; no persistent write per frame. Persist phase changes/checkpoints and flush on shutdown.
- Read APIs return snapshots and revision/cursor information. Dropped notification events must not lose durable work.

Diagnostics include queue/cache sizes, resource wait reasons, stage durations, retries and sanitized adapter versions. Logs omit cookies, credentials, signed URLs and raw command lines containing authentication selectors.

## 12. Output naming, deduplication and provenance

### 12.1 Naming

Proposed default:

```text
<safe title> [<provider>-<source id>] [clip-<start_ms>-<end_ms>-<mode>-<identity12>].<ext>
```

Normalize filenames for the platform; remove separators/control characters and bound the complete filename in UTF-8 bytes. If the filesystem limit cannot be established, use a conservative 180-byte basename plus extension. Preserve the complete title in metadata. The identity suffix is derived from the immutable render/source identity, not title text alone.

The plan shows filenames when identity is known; otherwise it clearly marks the suffix provisional. User titles must never become paths, URLs, shell expressions or FFmpeg filter fragments.

### 12.2 Identity and retry behavior

Separate three identities:

1. **Admission identity:** workspace-scoped idempotency key plus normalized request digest; replay returns the same admitted batch. Reusing the key with different intent is a conflict.
2. **Source preparation identity:** canonical provider/local identity, selected streams/profile, authentication context and acquisition policy; reuse additionally requires intact content evidence.
3. **Clip materialization identity:** prepared content digest, selected stream/timeline identity, requested and accepted effective bounds, accuracy, output profile/pipeline version and canonical destination directory.

Titles and UI ordering are not media-content identity. A matching existing materialization may be attached without rendering again; preview explicitly reports reuse and its existing filename. Do not create unexplained duplicate aliases solely because a later request uses another title.

Different requested ranges that produce identical fast effective bounds retain separate provenance; reuse must not erase either request's meaning. Changed pipeline semantics invalidate materialization reuse through a versioned identity.

### 12.3 Provenance contents

Store source/provider identity, sanitized canonical source reference, original title, source preparation fingerprint, requested/effective bounds, chapter origin, selected profile, clipping mode, output digest and tool/pipeline version evidence.

Provenance does not certify legal ownership or permanent source availability. It records what CamelClip processed.

Persist provenance in SQLite by default. Optional JSON export is explicit and privacy-filtered. Do not export browser selectors, secrets, raw signed URLs or unrelated local paths. Include a manifest schema version. A failed optional export produces a visible metadata warning and a retry action; it must not redownload or invalidate an already committed media asset.

## 13. Persistence, publication and migration

### 13.1 Proposed tables

| Table | Main purpose / constraints |
| --- | --- |
| `clip_plans` | ID, revision, expiry, normalized record/digest |
| `clip_batches` | ID, control state, source reference, request digest, unique workspace idempotency key |
| `clip_tasks` | ID, batch ID, ordinal, state, next attempt, materialization identity, unique batch/client segment identity |
| `clip_sources` | Preparation identity, state, private manifest, content/timeline evidence, accounting/checkpoint data |
| `clip_source_refs` | Durable consumer references for tasks/previews; unique source/consumer identity |
| `clip_bound_approvals` | Accepted effective-range revision/digest |
| `clip_publications` | Task/identity, intended final path, staged fingerprint, publication phase, asset ID |
| `asset_clip_provenance` | Asset-to-clip/source/range relationships, independent of removable task history |
| `clip_delivery_intents` | Materialization and snapshotted destination identity to admitted upload ID |

Use indexed relational columns for state/due time/batch order/materialization lookup, with versioned JSON for immutable request details where consistent with the repository. Avoid scanning every historical JSON record to find runnable work.

### 13.2 Atomic admission

One transaction admits a batch, ordered tasks, source references and destination snapshots, after validating plan revision/expiration and all inputs. Failure creates no partial batch. Alternative repositories must implement the same atomic port contract; do not claim SQLite-independent support while relying on hidden SQL behavior.

### 13.3 Publication protocol

Filesystem publication and a database commit cannot be one ordinary SQL transaction. Use a durable, idempotent protocol:

1. Verify the staged media and compute its digest.
2. Commit a publication intent with task identity, staged/final path and fingerprint.
3. Publish without overwriting an existing path, using same-filesystem atomic primitives already supported by the pipeline.
4. Commit the asset, provenance, task completion and selected upload intents in one repository transaction.
5. Reconcile unfinished intents at startup by checking actual files and hashes.

A crash after filesystem publication but before catalog commit must recover the same asset. An unrelated existing file is a conflict, even if duration/profile match. A byte-identical candidate can be reused only with matching intent/evidence. Never “fix” a conflict by deleting or overwriting user data.

The current process-local Published notification is not a durable publication journal. Clip recovery must not depend on that event having been delivered.

### 13.4 Migration requirements

- Allocate the next schema version and update maximum-version checks and diagnostics together.
- Migrate all supported old schemas transactionally; old jobs, assets, subscriptions, vaults and uploads are unchanged.
- Back up and test schema-5 fixtures with nonempty history before migration acceptance.
- Legacy whole-video records must not acquire fabricated clip ranges.
- Older executables must refuse a newer schema rather than silently dropping clip data.
- Introduce nullable/provenance relations without changing the meaning of existing asset IDs or download IDs.

## 14. Retry, cancellation and recovery

### 14.1 Failure scope

- Source acquisition failure affects only dependent unfinished tasks, not unrelated downloads.
- One render/verification failure does not poison siblings or the service.
- Storage failure stops new admission and publication safely; visible tasks retain recoverable evidence.
- Missing local source blocks dependent work; do not substitute a similarly named file.

### 14.2 Retry rules

Transient source/network failures reuse existing bounded retry policy. Validation, unsupported timing/codec, publication conflicts and rejected bounds need explicit user action. Automatic retries never modify time ranges or switch accuracy modes.

A task retry reuses a source only if the lease/content/timeline evidence remains valid. Completed siblings never consume another retry budget. Authentication refresh is an explicit change to acquisition credentials, not a change to the admitted editorial request.

### 14.3 Pause and shutdown

Pause prevents the next stage from starting; v1 need not suspend FFmpeg mid-process. The UI must distinguish “pause requested; finishing current stage” from a fully paused task. Cancel terminates/reaps the process group and leaves resumable source evidence without publishing an incomplete candidate.

Shutdown order: stop clip admission and preview requests; cancel/join clip and preparation workers; flush publication/checkpoint state; stop cloud/library services; close shared resources/repository through their owning lifecycle. All dependencies must still be alive while clip workers exit.

### 14.4 Recovery matrix

| Interrupted point | Required restart result |
| --- | --- |
| After admission, before source work | Resume the same batch/tasks |
| During acquisition | Validate/reuse resumable source parts or restart acquisition; no library clip yet |
| After source ready | Revalidate manifest/digest and reuse source |
| During fast-bound approval | Keep waiting; never auto-accept |
| During rendering | Discard/rebuild only the affected incomplete candidate |
| After render, before verification | Reuse candidate only with trusted fingerprint, then verify |
| After verification, before publish intent | Revalidate and recreate intent |
| After publish, before DB commit | Reconcile the intended existing file into one asset |
| After asset commit, before S3 admission finishes | Reconcile one delivery intent; no duplicate upload |
| During cache cleanup | Keep leased data; finish idempotent cleanup for unreferenced data |

## 15. S3 integration

- Selected S3 destinations are explicit per batch. Empty selection means local-only, independent of global AutoUpload.
- Each clip publishes and uploads independently. A source preparation is never an uploadable asset unless a separate whole-source save was explicitly requested; that save action is outside v1.
- Admission snapshots destination identity using the existing profile-change/conflict rules.
- Upload object keys include a stable clip identity and extension; titles may be a readable component but cannot be the collision boundary.
- Locked vault means waiting for unlock, not local clipping failure.
- Upload history removal must not erase delivery/provenance identity.
- Local completion is separate from delivery completion; no automatic local deletion is introduced here.
- Existing multipart cleanup, conditional publication and verification remain authoritative.

## 16. GUI specification

### 16.1 Entry points

- **Create clips** in the Add links flow and downloads header.
- **Create clips** beside an available library video/audio asset and in its details.
- A dedicated **Clips** workspace section for batches; do not mix batch IDs into legacy job IDs.

### 16.2 Editor

Source inspection panel: title, duration, source/provider, availability, selected source quality, acquisition mode, known/unknown size and relevant warnings.

Range table: ordinal, optional name, Start, End, derived Duration, chapter origin and validation feedback. Support add/remove/duplicate-row-with-new-bounds, keyboard editing and reorder. Duplicate exact ranges must still be resolved before admission.

Output panel: video/audio, codec/container or audio format, quality ceiling, precision mode, folder and optional S3 destinations. Fast-copy availability is explained before expensive processing; precise mode is clearly the default.

Plan summary: source acquisition once, estimated output duration, outputs/names, cache reuse, possible full-source fallback and resource/disk warnings. Admission is disabled for invalid or unresolved required fields.

### 16.3 Preview and fast approval

Preview is available only from an authorized local/prepared source or a separately generated bounded proxy. Before acquisition, the interface says preview is unavailable until preparation; do not pretend remote scrubbing is free.

The preview clock maps to the original source timeline. A browser-compatible proxy must carry explicit time mapping and must never be the authority used for final cut calculations. Unsupported preview codecs do not make the source unclipppable.

For fast-copy, show requested/effective intervals and extra content. Accepting one bounds revision cannot approve a later changed revision. Switching to precise creates a revised plan, not a hidden fallback.

### 16.4 Batch/results view

Show source-level acquisition and child rows with name, requested/effective time, mode, stage, progress, wait/error reason and delivery status. Ready rows expose Play, Reveal and Details. Keep Reveal next to the actual output, consistent with the current GUI behavior.

Support retry/cancel for selected children, cancel/pause/resume batch, provenance export and history removal. Explain that removing history keeps output files.

### 16.5 Accessibility and appearance

Support the existing system/light/dark palettes, visible keyboard focus, associated field labels and row-specific accessible action names. Status information cannot rely on color alone. On narrow windows, preserve range validation and actions without hiding the requested/effective distinction.

Do not repeatedly replace an actively edited range field during polling. Slow views resynchronize from snapshots. Long titles remain available in details/tooltips and do not force horizontal page overflow.

## 17. TUI and CLI specification

### 17.1 TUI

Add a Clips workspace screen with batch list and child detail view. Creating a batch is a guided source -> ranges/chapters -> profile -> review flow using the same planner. Fast-bound approval must be possible without GUI. Long-running source work must not block keyboard navigation; Escape/back navigates without canceling work unless the user explicitly requests cancellation.

### 17.2 Proposed CLI

```sh
# Inspection/plan only: metadata may be fetched; no media download/render.
camelclip clips plan --url 'https://www.youtube.com/watch?v=VIDEO_ID' \
  --range '00:12:30.000..00:15:10.000' \
  --range '00:42:00.000..00:43:20.000' \
  --accuracy precise --acquisition full_source --out './Clips' --json

# Admit an immutable plan. A retry with the same key returns the same batch.
camelclip clips create --plan-id PLAN_ID --revision 1 \
  --digest PLAN_DIGEST --idempotency-key my-request-001 --json

# Execute only the named batch in this workspace.
camelclip clips run BATCH_ID
camelclip clips list --limit 50 --json
# Pass --cursor NEXT_CURSOR for subsequent pages.
camelclip clips items BATCH_ID --json
camelclip clips approve-bounds BATCH_ID --revision 2 --digest DIGEST
camelclip clips retry TASK_ID
camelclip clips cancel BATCH_ID
camelclip clips provenance TASK_ID --json
```

These are proposed commands, not currently executable documentation. Implementation must publish exact help/flag names with its release. A manifest file is the preferred way to supply many named ranges; repeated `--range` is convenience syntax. Exactly one of URL, asset ID or source manifest reference is allowed.

- `plan` never performs media acquisition implicitly. If duration is unresolved, report the required preparation step.
- `create` persists only; it does not start unrelated queued work.
- `run BATCH_ID` runs only that batch and its explicitly bound deliveries, then exits when local work settles. Delivery waiting/unlock status is reported separately.
- `--wait-deliveries` may explicitly extend run until selected deliveries settle; locked credentials require the existing private password-descriptor mechanism or an explicit waiting result, not a hang with no explanation.
- Chapters are selected by inspected revision/index, not title regex.
- JSON output has schema/version fields, stable identifiers, machine-readable errors and no ANSI formatting or secrets.
- Suggested exit codes: 0 requested local work succeeded; 1 execution failure; 2 invalid input; 3 approval/authentication required; 4 some requested clips failed/canceled. Preserve the project's final global CLI convention if it conflicts, and document the mapping.
- The existing workspace ownership lock continues to prevent concurrent GUI/TUI/CLI writers. This feature does not add a parallel background owner.

## 18. GUI transport/API proposal

Extend the existing authenticated loopback transport; it is not a new public server API. Reuse token, Host/Origin, request-size, error and shutdown behavior.

| Operation | Proposed route | Semantics |
| --- | --- | --- |
| Resolve/create plan | `POST /api/clip-plans` | Metadata work and normalized reviewable plan |
| Get plan | `GET /api/clip-plans/{id}` | Includes revision/expiry and warnings |
| Admit | `POST /api/clip-batches` | Plan ID/revision plus idempotency key |
| Read batches | `GET /api/clip-batches?limit=50&cursor=OPAQUE_CURSOR` | Bounded snapshot |
| Read children | `GET /api/clip-batches/{id}/tasks` | Bounded per-task snapshot |
| Control/approve/retry | Existing action envelope or typed POST routes | One documented convention, shared service methods |
| Provenance | `GET /api/clip-tasks/{id}/provenance` | Sanitized data, not raw request dumps |
| Preview | `GET/HEAD /api/clip-preview/{opaque-id}` | Authorized, leased, bounded media reader |

Proposed admission envelope:

```json
{
  "schema_version": 1,
  "plan_id": "plan-example",
  "expected_revision": 1,
  "plan_digest": "sha256:example",
  "idempotency_key": "my-request-001"
}
```

Concurrent revision changes return a typed conflict. A preview reader accepts only opaque registered source/proxy IDs, supports validated byte ranges and cancellation, and never exposes arbitrary filesystem paths, directory listings or external redirects. Active reads hold cache leases. Bounded proxy generation uses the shared processing pool; request cancellation releases its resources.

## 19. Errors and user-visible recovery

| Code | Meaning | User action / retry |
| --- | --- | --- |
| `clip_invalid_range` | Syntax/order/duration invalid | Correct field; no automatic retry |
| `clip_out_of_bounds` | Range exceeds authoritative timeline | Review revised plan |
| `clip_source_unsupported` | Playlist/live/DRM/unsupported source | Choose a supported finite item |
| `clip_source_changed` | Source no longer matches admitted identity | Inspect and create a new revision |
| `clip_source_missing` | Required local/prepared data missing | Restore/select/reacquire explicitly |
| `clip_timeline_unsupported` | Cannot prove timing map | Choose another source; no precise claim |
| `clip_fast_unavailable` | No compatible independently decodable copy cut | Choose precise mode |
| `clip_approval_required` | Effective bounds or fallback need acceptance | Review and approve revision |
| `clip_disk_blocked` | Staging/free-space limit reached | Free space/change limits; resume |
| `clip_profile_unsupported` | Codec/container/HDR capability unsupported | Choose supported profile/source |
| `clip_verification_failed` | Decode/timeline/profile contract failed | Inspect details; bounded retry if appropriate |
| `clip_publication_conflict` | Different file occupies final path | Choose another output directory/name through a new plan |
| `clip_plan_expired` | Plan is too old to admit | Reinspect/review |
| `clip_revision_conflict` | Concurrent/stale control request | Refresh snapshot |

Use existing generic validation, canceled, adapter-contract and persistence codes where sufficient; do not fork equivalent error taxonomies unnecessarily. Internal command stderr must be sanitized before display/export.

## 20. Media adapter implementation guidance

### 20.1 Upstream facts versus CamelClip requirements

FFmpeg distinguishes input seeking from output seeking. Accurate input seeking with transcoding can decode/discard material before the requested position; stream-copy does not provide the same guarantee. yt-dlp exposes section acquisition and a keyframe-at-cuts option that involves re-encoding. These capabilities are building blocks, not evidence that a CamelClip output meets the timing contract. See the primary references in section 24.

### 20.2 Required implementation approach

- Build subprocess argument arrays, never shell commands from URLs/titles/times.
- Resolve a finite source first; FFmpeg receives approved local source descriptors, not arbitrary user-provided filter graphs or unvalidated nested network playlists.
- Maintain the original presentation-time mapping through optional merge/normalization. A preprocessing step that changes timing must produce a verified mapping or fail.
- Precise mode deliberately encodes selected streams and accounts for seek pre-roll and encoder delay. Avoid a generic conversion helper that opportunistically copies streams.
- Fast mode plans and approves decodable boundaries, copies only compatible streams and then fully verifies the result.
- Partial acquisition must include sufficient context and an explicit source-time origin. A downloaded section starting at output time zero is not assumed to start at requested source time.
- Version-sensitive options and format capability decisions belong to adapters, with bounded probes and sanitized diagnostics.
- A successful FFmpeg exit is necessary but insufficient; publication requires clip-specific verification.

No single example FFmpeg command is normative. Command templates must be selected only after reference fixtures prove source offsets, edit lists, B-frames, VFR, audio delay and supported codec/container combinations.

## 21. Verification and test strategy

### 21.1 Unit and contract tests

- Time parsing/canonicalization, overflow, Unicode titles, exact duplicates, overlap, order preservation and all limits.
- Provider/local identity, immutable revision admission, idempotency replay/conflict, and chapter expansion.
- Separate accurate/fast contracts, unapproved bounds, no hidden mode/fallback change.
- Renderer/source observer contracts: finite metrics, stable task identity, serialization, no late mutation after return/panic.
- Shared resource limits, no permit deadlock, bounded metadata work, fair admission between normal downloads and clips.
- Source references, active preview leases, cancellation of one sibling and cleanup refusal while leased.
- No S3 admission for private sources or failed candidates; local-only overrides global AutoUpload.

### 21.2 Deterministic media fixtures

Generate local reference media with known visual frame IDs and audio markers; verify selected source content, not just the output's duration.

| Fixture | Required assertion |
| --- | --- |
| CFR, cuts on/between frames | First/last selected frame matches precise rules |
| VFR with long/short frame intervals | Local frame evidence used; average FPS cannot hide drift |
| Long GOP and B-frames | Fast expansion shown; precise does not leak pre-roll frames |
| Nonzero/negative timestamps and edit lists | Normalized source timeline remains correct |
| Audio encoder delay and audio-leading/video-leading source | Boundary and introduced A/V offset meet thresholds |
| Separate audio/video tracks | Shared acquisition/merge preserves timing |
| End exactly at source end | No fabricated extension, no truncated valid tail |
| Minimum 1-second clip | No inherited whole-media 3-second tolerance |
| Overlapping and out-of-order ranges | Separate correct outputs and stable ordering |
| Unsupported HDR/codec/timing | Clear failure; no mislabeled successful output |
| Fast output with incompatible profile | No implicit transcode |
| Unicode/spaced filenames and path collision | Valid output; unrelated file unchanged |
| Missing, replaced, symlinked or modified local source | No unintended input/output replacement |

Use known frame/audio landmarks to test boundary correctness. A fully decodable but wrongly positioned clip must fail tests.

### 21.3 Fault injection and restart

Kill the owning process at every recovery-matrix point, reopen the same temporary workspace and assert exactly-once asset/publication/delivery identity. Cover full disk, database write failure, permission failure, FFmpeg nonzero exit/panic, corrupted source cache, expired source URL, locked vault, unavailable S3, and cancellation during each stage.

Test wrong preexisting output and same-content recovery independently. Include a crash after publication before the DB transaction, and after optional manifest export failure. Never fill the real host disk or use real browser data/cloud keys.

### 21.4 Frontend acceptance

- GUI supports manual and chapter ranges, keyboard correction, requested/effective fast bounds, both themes, row actions, partial failure and restart.
- TUI can create/review/approve/cancel without GUI and keeps navigation responsive.
- CLI JSON matches the same normalized plan and errors; one-shot commands do not run unrelated work.
- Existing GUI progress, whole-video downloads, playlists, library, vault and installer tests remain green.
- Preview test proves authenticated bounded reads and cache-lease cleanup behavior.

### 21.5 Performance acceptance

- A 100-interval batch uses one source preparation and a bounded worker count, not 100 simultaneous processes.
- No whole source or output is buffered in RAM by the application; hashing/verification streams data.
- Batch/task listing is paginated, with source progress represented once.
- Metadata/progress notifications do not cause per-frame SQLite writes.
- Benchmark plan normalization/admission for 100 intervals and task listing with 10,000 historical records; record hardware and results instead of promising a universal latency.
- Partial acquisition is enabled only after fixture parity with full-source acquisition and observed bandwidth accounting. If it increases work for a source type, capability policy can choose full-source honestly.

## 22. Rollout and implementation PRs

| PR | Deliverable | Gate |
| --- | --- | --- |
| 1 | Time/domain model, immutable planner and typed ports | Parsing/identity/compatibility contracts; no runtime clipping |
| 2 | Transactional migration, admission, source/task/publication records | Upgrade/reopen/rollback fixtures preserve all existing data |
| 3 | Shared resource authority and source coordinator | Legacy limits unchanged; one source preparation; cancellation/leases proven |
| 4 | Precise local-source renderer and clip verifier | CFR/VFR/audio-delay/reference-frame acceptance |
| 5 | Crash-safe publication, provenance and recovery | Fault matrix; no overwrite/duplicate asset |
| 6 | URL acquisition, chapters and full-source disclosure | Existing authentication/quality/cancellation preserved |
| 7 | GUI/TUI/CLI editor and batch views | Same service contracts; usable local precise increment |
| 8 | Fast-bound planning/approval and supported copy paths | Independent-decode/timing evidence and no hidden fallback |
| 9 | Optional S3 and private preview/provenance export | Per-clip idempotent delivery; authorization/leases |
| 10 | Optional section acquisition optimization | Full-source parity, timing origin and rate/cancel tests |

The exact PR split may change, but no UI success state may precede its correctness/recovery gate. Source-only review merges do not require publishing a new release. Application/installer/release changes require the task's explicit distribution scope, not an assumption made from this specification.

### 22.1 Feature exposure

Keep new commands/UI entry points unavailable until required ports are wired. Do not hide active durable clip work after an upgrade/configuration change. When a capability is unavailable, read-only history and recovery explanations remain accessible.

### 22.2 Definition of done for complete v1

- [ ] All scope items in 2.1 are implemented or a narrower increment is explicitly named/documented.
- [ ] Precise and fast semantics match their separate acceptance contracts.
- [ ] One source is shared and ordinary download/resource limits remain authoritative.
- [ ] Existing downloader Result/Published contract remains unchanged.
- [ ] Partial completion, approval, cancellation and restart are deterministic.
- [ ] Every completed clip is verified, cataloged, traceable and revealable as its actual file.
- [ ] Selected S3 delivery is independent and idempotent; original/private sources never auto-upload.
- [ ] Legacy settings/jobs/assets/playlists/vaults survive migrations.
- [ ] English public docs/help and both application localizations are updated.
- [ ] Required local and hosted CI evidence is recorded, with unavailable platform checks stated honestly.
- [ ] No release/installers are changed merely by implementing this specification.

## 23. Decisions requiring a separate future specification

Do not silently include these while implementing v1:

- Automatic clipping rules on playlist subscriptions and their interaction with whole-video admission.
- Cloud-only source restoration and archive-retention leases.
- Subtitle clipping, subtitle-language selection and embedded subtitle stream behavior.
- Montage construction from intervals, multitrack choices and HDR/tone-mapping profiles.
- Frame editing, proxies with rich scrubbing/waveforms, and cross-source projects.
- Server multi-user authorization, remote workers and distributed cache ownership.

## 24. References and evidence boundaries

Repository links above describe the inspected baseline, not a guarantee that these files will retain identical internals during implementation.

Primary upstream references consulted on 2026-10-23:

1. [FFmpeg official documentation source](https://github.com/FFmpeg/FFmpeg/blob/master/doc/ffmpeg.texi) — input/output option ordering, seeking and accurate-seek behavior. Runtime command behavior must still be verified against supported installed versions.
2. [yt-dlp official download options](https://github.com/yt-dlp/yt-dlp#download-options) — section acquisition capabilities.
3. [yt-dlp official post-processing options](https://github.com/yt-dlp/yt-dlp#post-processing-options) — chapter splitting and keyframe-at-cuts behavior.
4. [CamelClip architecture](../architecture.md), [GUI](../gui.md), [CLI](../cli.md), [playlist contracts](../playlists.md), and [testing](../testing.md).

All thresholds, state models, limits, proposed interfaces and rollout decisions in this document are CamelClip design requirements. They are not claims that upstream tools or the current application already implement them.


## 25. Independent Core: ownership and dependency rules

### 25.1 Non-negotiable boundary

The complete feature MUST run through an injected application service with fake adapters in a Go test that imports no UI, bootstrap, SQL driver or real media executable. GUI, TUI, CLI and a future server are clients of this service. They MUST NOT decide whether a range is valid, approve a fallback implicitly, assign a terminal state, deduplicate a source, allocate independent media workers or enqueue uploads themselves.

The domain package owns deterministic value validation, canonical editorial intent, transition guards, capability requirements, materialization identity inputs, approval rules and batch aggregation. The application layer owns orchestration over ports, lifetimes, retries, resource acquisition, idempotent admission, publication reconciliation and delivery intents. Adapters produce facts; the domain/application layers decide whether those facts satisfy the admitted request.

```text
GUI / TUI / CLI / future server
              |
              v
application.ClipService -----> core clip rules and values
              |
              +---- repository transaction port <---- SQLite adapter
              +---- source port                 <---- yt-dlp/local adapter
              +---- media/timeline port         <---- FFmpeg/ffprobe adapter
              +---- publication/cache port      <---- filesystem adapter
              +---- shared resource port        <---- workspace resource authority
              +---- delivery port               <---- existing CloudService

bootstrap constructs dependencies; no arrow points from Core to a frontend.
```

### 25.2 Proposed file map

| Location | Responsibility | Explicitly excluded |
| --- | --- | --- |
| `internal/core/clips.go` | Requests, plans, task/source values, range and profile types | SQL rows, HTTP handlers, process arguments |
| `internal/core/clip_validation.go` | Pure normalization, bounded time parsing and validation | Network inspection or filesystem stat |
| `internal/core/clip_policy.go` | State guards, approvals, aggregate state, retry classification | Timers and goroutines |
| `internal/core/clip_identity.go` | Versioned canonical identity input and digest rules | Reading arbitrary files |
| `internal/application/clip_ports.go` | Narrow service dependencies and transaction operations | Concrete storage types |
| `internal/application/clip_service.go` | Public commands, lifetime, admission and read models | Rendering implementation |
| `internal/application/clip_planner.go` | Compose inspected facts with pure domain planning | Subprocess execution |
| `internal/application/clip_sources.go` | Shared preparation, references, recovery | UI state |
| `internal/application/clip_executor.go` | Attempt state progression and verification policy | Filename-based success guessing |
| `internal/application/clip_publication.go` | Durable publication protocol and reconciliation | Direct SQL access |
| `internal/application/clip_delivery.go` | Explicit destination intents and idempotent handoff | Secrets in clip records |
| `internal/storage/sqlite/clips*.go` | Atomic port implementations and indexed queries | Editorial decisions |
| `internal/media/ffmpeg/` | Bounded timeline probes, rendering, decode evidence | Batch control or S3 admission |
| `internal/downloader/ytdlp/` | Remote source inspection/acquisition integration | Publishing private source assets |
| `internal/bootstrap/` | Tool resolution, shared ownership and injection | Duplicate business validation |

Names MAY be adjusted to repository conventions; responsibilities MUST remain separated. Extract existing private pipeline helpers in small steps, preserving their whole-video callers. Do not move the entire existing verified pipeline into an oversized generic media service.

### 25.3 Architecture gates

Add dependency checks using Go package metadata, covering production and test-only imports separately:

- `internal/core` uses standard-library packages only; no application, UI, storage, bootstrap or adapter import.
- `internal/application` imports domain contracts, not concrete SQLite, yt-dlp, FFmpeg or frontend implementations.
- Frontends use application contracts/read models and core values; concrete adapter construction stays in bootstrap.
- A headless contract suite constructs ClipService with in-memory fakes and executes planning, admission, cancellation, recovery and delivery.
- Public errors and DTOs contain no `*sql.Tx`, command objects, GUI models, raw cookies or SDK credentials.

An independently usable Core is a runtime/dependency property, not a requirement to publish a separate Go module. Cross-module/public SDK stability is outside v1.

## 26. Public service contract and command semantics

### 26.1 Service surface

Use explicit commands rather than exposing mutable task pointers. Proposed application-facing shape:

```go
type ClipService interface {
    Inspect(context.Context, ClipInspectCommand) (ClipInspection, error)
    Plan(context.Context, ClipPlanCommand) (ClipPlan, error)
    Admit(context.Context, ClipAdmitCommand) (ClipBatchSnapshot, error)
    ApproveBounds(context.Context, ClipApproveCommand) (ClipBatchSnapshot, error)
    Control(context.Context, ClipControlCommand) (ClipControlResult, error)
    GetBatch(context.Context, string) (ClipBatchSnapshot, error)
    ListBatches(context.Context, ClipBatchQuery) (ClipBatchPage, error)
    ListTasks(context.Context, ClipTaskQuery) (ClipTaskPage, error)
    GetProvenance(context.Context, string) (ClipProvenanceView, error)
    Close() error
}
```

Inspect is side-effect limited: bounded metadata reads and optional explicitly selected authentication access; no media acquisition. Plan references an inspection ID/revision, canonicalizes ranges, resolves selected output defaults, snapshots capability requirements and persists an immutable review record. Updating a draft creates a new plan revision and invalidates the old approval digest. Admit references plan ID, expected revision, digest and a client-generated idempotency key. It does not accept replacement hidden request fields.

The source reference is a discriminated union: `url` with a sanitized HTTP(S) reference, or `asset` with a catalog ID. Exactly one branch is allowed. Local paths supplied by future remote clients MUST go through a separate authorized asset-import use case; no arbitrary-path field is added to this contract.

### 26.2 Request identity and replay

- Idempotency keys are workspace scoped, 1–128 ASCII characters from letters, digits, `-`, `_`, `.`, `:`. They are not secrets.
- Admission first checks an existing key against the submitted plan identity/digest. An identical replay returns the original batch even if the plan has since expired. A different digest returns a conflict.
- A new admission validates expiration, expected revision, approval coverage, capabilities and workspace capacity inside the atomic admission operation where applicable.
- HTTP connection loss or caller-context cancellation after transaction commit does not cancel admitted work. The caller retries the same key or reads the batch.
- Before commit, canceled commands MUST make no durable admission. If the storage adapter cannot establish whether a commit succeeded, return a typed outcome-unknown error with the idempotency key; never claim definitely rejected.
- Replay evidence survives terminal-history removal. It can be compacted into a tombstone containing key, request digest and batch identity; replay then returns an explicit archived result, never admits duplicate work.

### 26.3 Control revision and atomicity

`ClipControlCommand` carries batch ID, expected **control revision**, action and either all tasks or a bounded unique list of task IDs. Worker progress has a separate revision and does not cause every user command to conflict. Control revision changes for user controls, new approval requirements and editorially significant source changes.

Validate every selected task and batch membership first; apply the selection atomically or reject the entire command with per-field details. A resume/pause/cancel repeated against its already achieved state is a no-op, subject to expected revision semantics. Retry of a `done` task is rejected; retry increments the attempt epoch and preserves previous attempt diagnostics.

A control response distinguishes `applied`, `already_applied`, and `pending_stage_completion`. Batch pause prevents new stages and lets an active render/verification stage settle; cancellation signals current processes. Resume only removes the user's pause; it does not accept bounds, unlock a vault or override a disk block.

### 26.4 Call ownership and lifetime

Inspection/planning follow the caller context and service lifetime. Admitted durable execution follows workspace lifetime; it is not owned by a particular UI window or CLI response. A one-shot CLI admission persists a queued batch and exits without claiming it will execute in the background. `clips run`/workspace worker mode explicitly owns execution while open; no daemon is invented.

`Close` is idempotent, rejects new commands with existing `application.ErrClosed`, cancels and joins active calls/workers, flushes durable checkpoints and returns joined shutdown errors. A timed-out frontend wait does not authorize closing storage while a worker is alive. Workspace single-instance locking continues to cover all interfaces and clip workers.

### 26.5 Read models and events

Snapshots are copies: modifying returned slices/maps cannot mutate service state. Include IDs, control/read revision, task ordinal, requested/effective bounds, attempt number, typed wait/error reason, asset ID and phase metrics. Source acquisition appears once per shared preparation, with references from batches. Private locations remain internal.

List requests use an opaque stable cursor and a limit of 1–200 (default 50); order by immutable creation sequence plus ID. Task order within a batch is original ordinal. Do not use mutable progress timestamps as cursor keys. Counts and items MUST specify snapshot consistency; v1 uses a repository read transaction for each page.

Notifications are advisory, bounded and coalesced, carrying entity ID and read revision. Subscribers re-read snapshots after gaps/reconnect. Durable terminal state and publication never depend on subscriber delivery. Public JSON has `schema_version: 1`; unknown enum values render as unsupported/read-only, not as a guessed success.

## 27. Canonical values, policy and capability negotiation

### 27.1 Resolve every default before admission

A stored plan contains explicit accuracy, acquisition policy, fallback permission, selected streams, codec/container, quality/height policy, audio profile, output root, source identity, optional destination snapshots, naming policy version, pipeline semantic version and required capabilities. Settings changed after admission affect future plans only. Runtime rate/concurrency/disk limits may change without changing editorial intent.

Use a deterministic canonical encoder with a fixed field order, UTF-8 encoding and integer times; never hash unordered Go maps or arbitrary transport JSON. Sort only semantically unordered sets such as destination IDs; preserve segment ordinal. Exclude transient progress, display-localized messages, signed URLs and secrets. Include a domain/version prefix in each digest. Persist canonical input bytes or an equivalent versioned immutable record for audit.

Plan digest includes reviewed names, order, destinations, fallback and output directory. Render-content digest excludes titles/destination/S3, but includes source content/timeline, streams, requested/approved bounds, profiles and pipeline semantics. Materialization identity adds the canonical destination directory. These distinct digests MUST NOT be used interchangeably.

### 27.2 Capabilities are facts, policy stays in Core

The injected media adapter reports capabilities per pinned toolchain: precise video/audio support, fast-copy codec/container pairs, timeline mapping support, decode verification, local preview suitability and section-acquisition support. Capabilities include a versioned fingerprint and sanitized diagnostic reason when unavailable.

Core intersects capabilities with requested semantics; adapters cannot silently substitute a profile. A toolchain update after planning triggers a compatibility check at attempt start. If required semantics remain supported, use the new toolchain and record actual versions; otherwise wait for a compatible toolchain or require replanning. Pipeline semantic-version changes invalidate materialization reuse even if filenames or tool versions look similar.

No updater or tool installation belongs inside the render call. Tool resolution is injected as an immutable per-attempt snapshot of executable paths and versions. Running attempts keep their snapshot; updates must not remove executables still leased by a process. This specification consumes that resolver contract but does not implement the proposed dependency updater.

### 27.3 Failure contract

Reuse `core.Failure`, `core.CodeOf` and `core.Retryable`; extend stable codes only where the user recovery action is meaningfully distinct. Clip-specific codes in section 19 are extensions of that mechanism, not a second error wrapper. Validation errors additionally identify stable field paths such as `segments[2].range.end_ms`.

Every persisted failure contains code, safe message, retry class, attempt, stage and occurrence time. Optional structured details are bounded and sanitized. Retry class is `automatic`, `user_action`, or `terminal`; only `automatic` sets Temporary. Raw stderr is bounded diagnostic evidence, never the source of frontend control logic. Unknown errors default to a safe failed attempt, not an infinite automatic retry.

Clock and ID generation are injected into application components; domain functions take explicit time/input values. Automatic retries use capped exponential backoff with jitter and existing configured retry limits. User-action waits do not consume attempts or spin on every scheduler tick. On restart honor persisted next-attempt time; clamp pathological clock changes with a bounded delay and record the adjustment.

## 28. Durable state, transactions and attempt fencing

### 28.1 Repository operations, not generic Save calls

A narrow ClipRepository port MUST expose these atomic operations (names illustrative):

| Operation | Atomic precondition and effect |
| --- | --- |
| `AdmitPlan` | Replay/expiry/revision/capacity checks; insert batch, tasks, source references, destination snapshots and key evidence together |
| `ClaimSource` | Eligible due preparation, expected revision; assign new attempt epoch and owner token |
| `ClaimTask` | Eligible unpaused task, ready compatible source and valid approvals; assign epoch and owner token |
| `CheckpointAttempt` | Match owner token/epoch; update state and phase evidence, rejecting stale workers |
| `ApplyControl` | Match control revision; validate all selected tasks; atomically persist control intent |
| `AcceptBounds` | Match source fingerprint, task set and bounds digest; persist acceptance and advance control revision |
| `RecordPublicationIntent` | Verified candidate and current owner; reserve materialization/final-path identity |
| `CommitClipPublication` | Current intent and verified final evidence; asset, provenance link, task completion and delivery outbox together |
| `CompleteDeliveryIntent` | Link one destination/materialization intent to its existing/admitted upload identity |
| `ReconcileInterrupted` | Read evidence and CAS transitions; no assumption that a previous callback completed |

Transactions contain no subprocess execution, long hashing, external uploads or UI callbacks. Compute expensive evidence outside a transaction, then revalidate identities/revisions when committing. Backend busy/transaction retry is bounded and reruns only side-effect-free transactional code.

### 28.2 Attempts and stale callbacks

Every source/task attempt has a monotonically increasing epoch and opaque ownership token. All progress, errors and publication actions identify entity, epoch and token. Tokens are execution fences, not user credentials. Late callbacks from a canceled/retried attempt are ignored and logged once; they cannot overwrite the new attempt or mark it done.

Adapters serialize callbacks, join them before returning, honor cancellation and reap children. A contract wrapper validates IDs, allowed phase transitions and finite nonnegative metrics; catches panics at the adapter boundary; and closes event acceptance before return. A panic/contract violation fails the attempt and quarantines its candidate. Subscriber callbacks cannot reenter a service while its mutex/transaction is held.

### 28.3 State refinements and precedence

Section 10 is a state vocabulary, not permission to jump between arbitrary states. Workers use guarded transitions:

```text
queued -> waiting_source -> waiting_processing -> rendering
       -> waiting_verification -> verifying -> publishing -> done
```

The path may enter `waiting_range_approval` after source readiness and before rendering. Paused/wait reasons preserve a durable resume phase. Retry never returns `done` to an active state. A failed source leaves children in `waiting_source` with a source-failure reason while automatic source retries remain; exhausted source failure settles dependent unfinished tasks as failed without changing completed siblings.

Cancellation has a transaction linearization point. Before publication intent, an accepted cancel prevents creating an intent. After an intent exists, reconciliation MUST finish deciding whether publication committed before honoring cancellation: an already committed asset yields `done`; a safely unpublished candidate can yield `canceled`. The control response reports this race explicitly. Never report canceled while leaving an undisclosed published output.

For unsettled aggregate status, precedence is: outstanding cancel intent; outstanding user pause; active work; unresolved approval/action wait. Once all tasks settle, derive the outcome from their results and whether cancellation actually prevented work; a late cancel/pause cannot override an entirely completed batch. Mixed done/canceled tasks without failures settle as canceled; zero successful tasks with any failure settle as failed; successful tasks plus failures settle as partial_failure unless batch cancellation actually canceled remaining work. A batch control field preserves user intent independently of derived counts. Empty batches are impossible. Counts partition total tasks; no task is counted twice as active and waiting.

### 28.4 Database constraints and retention

Required unique constraints: admission key; `(batch_id, ordinal)`; `(batch_id, client_segment_id)`; preparation identity among reusable preparations; publication intent per materialization; final reserved path; and delivery identity per materialization/destination snapshot. Use an explicit nullable active-preparation key or equivalent to permit historical failed attempts without defeating deduplication.

Indexes cover `(state, next_attempt, creation_sequence)`, batch task order, source references, unfinished publication phase and undelivered outbox entries. Foreign-key deletion rules MUST preserve provenance and idempotency evidence when display history is forgotten. Asset forget/delete checks MUST consider active source leases; an input cannot be removed while rendering. Missing externally deleted files become a typed source-missing condition.

## 29. Shared source lifecycle, resources and disk pressure

### 29.1 Resource authority extraction

The inspected `application.resources` currently uses integer download job IDs, private constructors and rank/control maps. Clips MUST NOT use ID 0, negative IDs, hashes truncated to int, or fake rows in `jobs` to join that mechanism.

Introduce a comparable typed owner key such as `{Kind: download|clip_source|clip_task, ID: string}` and a workspace-owned resource authority. Adapt the existing queue through a wrapper preserving ordering, rate reservation, pause and disk behavior. Keep existing `core.Resources`/`WithResources` for legacy adapters unless a separately tested compatibility change is necessary. Add metadata permits as a distinct bounded class (default two), not extra download permits.

Scheduling uses explicit priority then FIFO sequence, with bounded aging so a continuous stream of new ordinary jobs cannot starve clips or vice versa. One source acquisition consumes one network permit and its effective rate budget regardless of dependent task count. Rendering and full-decode verification consume processing permits sequentially. Waiting for source, approval, vault unlock or delivery consumes no media permit.

Never hold the resource mutex across repository calls, observers or subprocess shutdown. Permit release is idempotent and owner checked. Changing limits below active use stops new grants; it does not issue duplicate releases or pretend running children stopped.

### 29.2 Source reference and lease accounting

Distinguish durable consumer references (unfinished tasks needing the source) from active read leases (running render, probe, preview). Terminal tasks release preparation references after publication/recovery evidence no longer requires source bytes. Paused tasks retain references. Preview leases expire on closed streams and have bounded lifetime; they cannot keep the cache alive indefinitely after a disconnected client.

On concurrent compatible admission, one transaction elects/attaches the active preparation. If its worker dies, recovery assigns a new epoch after reconciling process locks; no competing acquisition is started while an orphan child still writes staging. Content validation after acquisition promotes a provisional preparation key into a manifest with digest and timeline evidence. Merely finding a matching filename is never a ready-source proof.

Credential context is an opaque security-service reference with revision, not a browser name or cookie value in the key. Reuse is scoped to that reference and workspace. Where browser authentication cannot establish a stable context identity, conservatively disable cross-batch private-source reuse; within-batch sharing remains mandatory. Refreshing credentials never grants access to another principal's cache.

### 29.3 Disk reservations and cleanup

Reserve estimated staging/source/output bytes before starting a stage; unknown estimates require a configured free-space floor and continuous actual-byte accounting. Reservations are conservative scheduling estimates, not a guarantee against another process filling the disk. Recheck before render, verification and publication. ENOSPC closes/reaps writers, preserves attributable partial data and enters a disk-blocked wait without rapid retries.

Count prepared streams, merge copies, candidates, final files awaiting reconciliation and cleanup-pending bytes. Source cache budget is separate from user library retention; cache cleanup never deletes published clips or local input assets. Cleanup uses owned-directory manifests and leases, rejects symlink escapes, and rechecks ownership immediately before removal. Do not recursively delete a user-supplied output directory.

Local input assets are immutable by contract but external tools may modify them. Capture file identity/size/mtime and digest, pin an open descriptor where supported, and recheck before/after rendering. Changed evidence invalidates the candidate. If the platform/adapter cannot provide a stable read snapshot, make an explicit accounted private copy before precise processing or return a source-changed failure; do not rely on stat alone.

## 30. Media algorithm and verification evidence

### 30.1 Deterministic baseline pipeline

For each compatible batch:

1. Resolve canonical source and selected stream expectations using bounded metadata work.
2. Acquire once to private storage, or validate the selected local asset.
3. Probe streams and establish authoritative normalized presentation timeline.
4. Validate **all** requested ranges against that timeline before any child renders.
5. For fast mode, derive all effective bounds and request explicit approval.
6. For each eligible task, render one candidate into its private attempt directory.
7. Fully decode candidate and validate profile, timeline evidence and synchronization.
8. Hash candidate, record intent, publish and commit catalog/provenance/delivery outbox.
9. Release consumer references and clean only eligible private data.

If authoritative metadata invalidates any admitted range, block the whole batch before rendering under `clip_out_of_bounds`, identifying every invalid segment. A new reviewed plan is required. This resolves the optional policy in section 5.3: v1 does not partially execute a batch with invalid ranges.

### 30.2 Timeline mapping is explicit data

The normalized source origin is the earliest valid presentation start among selected streams after interpreting container edit lists. Persist original stream time bases, start offsets, selected stream IDs and the mapping to that origin. Relative A/V offsets survive normalization. Represent adapter-level timestamps as rationals or integer ticks plus time base; rounding to public milliseconds is for bounds/display, not evidence comparison.

When using a seek optimization, record the seek origin and decode sufficient pre-roll. Apply range selection against mapped source timestamps, not output counters. A merge that resets each stream independently is prohibited unless it preserves and proves the mapping. Discontinuous/ambiguous timing is rejected for precise v1.

### 30.3 Renderer result and verifier result

`ClipStagedResult` contains an opaque owned candidate reference, attempt identity, actual stream/profile data, source-map digest, selected source boundary evidence and sanitized toolchain evidence. It is not a published path and cannot itself cause a done state.

A separate verifier port returns `ClipVerificationEvidence`: full-decode outcome, output digest/size, first/last source selection evidence, neighboring source frame durations, audio trim/encoder-delay evidence, actual output timeline, measured introduced A/V offset and a verifier semantic version. Core/application validates this evidence against the admitted mode and thresholds before publication. A boolean `verified=true` or an output duration alone is insufficient.

Runtime verification establishes structural/decode/timestamp consistency using independently probed output data and retained source-selection evidence. Fixture tests additionally establish **content** correspondence using known frame/audio markers. Do not claim a runtime cryptographic proof of frame origin from container timestamps. If the adapter cannot establish required boundary evidence for a supported path, that path is unavailable rather than accepted with a looser tolerance.

### 30.4 Output profile matrix

First vertical slice MUST support precise H.264/AAC MP4 video and precise audio-only output for explicitly tested existing audio profiles. Video requires selected video and audio streams, as in section 6.3. Preserve source resolution up to the reviewed height policy, preserve aspect ratio, and do not silently upscale. Audio-only does not acquire video when the provider can supply the selected audio independently.

Expose additional codec/container combinations only after the same fixture suite passes for them. Store the resolved encoder settings as a named versioned profile; user-supplied codec flags/filters are not accepted. A user preference `best` is resolved to explicit stream/profile decisions before final render identity is established. Hardware encoding is optional later; it cannot silently change a precise profile's evidence contract.

Fast-copy capability is per selected stream/container combination. Bound proposals contain requested interval, effective source interval, expansion at each end, decodability evidence and digest. Approval binds that entire set and prepared-source fingerprint. Any changed proposal invalidates approval. If any fast task fails to find legal bounds, block batch fast admission until the user replans; do not render only the easy siblings silently.

### 30.5 Partial acquisition remains an optimization

`prefer_sections` is unavailable in the first shipping slice. The capability response says so before admission. Later enable it only when the adapter returns a trustworthy source-origin map and sufficient pre/post-roll, with fixture parity against full-source rendering. Download savings are reported from measured bytes, not inferred from requested duration.

An expired signed URL can be refreshed for the same preparation identity. A changed provider item, stream semantics or authoritative timeline cannot. If full-source fallback was not approved, it needs a new source-policy approval digest; accepting fast bounds does not authorize extra acquisition.

## 31. Publication, reuse and S3: precise integration rules

### 31.1 Filesystem transaction boundary

Stage the final candidate on the destination filesystem before publishing. If source cache is on another device, account for the transfer into a private destination staging directory; do not turn a cross-device rename failure into an overwrite copy. Atomic non-overwriting publication is mandatory; platforms lacking a supported primitive fail with a capability error.

Intent records include candidate digest/size, owned staging reference, final reserved path, materialization identity, epoch and intended asset ID. Flush candidate data and durable intent before publication; flush affected directory metadata where the platform supports it. Power-loss durability limitations must be documented for supported filesystems, not silently equated with process-crash tests.

On a conflicting final path, compare only after establishing matching recorded intent. An identical hash without ownership/intent is not permission to adopt an unrelated file. Cleanup after commit failure is performed by reconciliation, not an eager compensating deletion that could remove a valid user-visible result.

### 31.2 Materialization reuse and provenance multiplicity

A materialization has one canonical local asset per destination identity. Multiple tasks may link to it; each retains its own request and provenance relation. Task completion by reuse requires revalidating current asset existence, hash and compatibility evidence. Missing/mutated existing materialization is blocked or explicitly rebuilt via a new attempt under the same no-overwrite rules.

The existing catalog's path-based `putAssetTx` merge is not sufficient by itself: clip commit must assert expected materialization/asset identity and preserve existing whole-video `DownloadID` semantics. Clip assets do not receive fabricated ordinary job IDs. Library scan encountering an already published clip must preserve its provenance and verified identity rather than replacing it with generic scan metadata.

### 31.3 Delivery outbox and existing cloud service

Local publication atomically records selected delivery intents; it does not call S3 within the transaction. A dispatcher invokes an idempotent CloudService enqueue port, then CAS-links the returned upload job. If enqueue succeeded but the link transaction failed, replay finds the same upload identity.

Review current cloud enqueue/global AutoUpload paths before wiring: a clip commit must carry an explicit delivery policy so an empty destination list cannot be overridden by global AutoUpload. An already reused asset gains only explicitly requested missing delivery intents. Multiple task links do not imply multiple uploads to the same destination snapshot.

Reuse the existing upload verification contract for uploads; successful local clipping does not strengthen S3 verification from size-only to full checksum automatically. Destination profile changes block delivery according to existing revision rules; they do not invalidate local output or silently target a new bucket. Vault lock requires explicit unlock and does not consume render retries.

## 32. Frontend and future server obligations

Frontends format domain errors and send the same commands. They may provide responsive provisional input hints, but MUST display authoritative Core validation on review/admission. Only Core/application records success, effective bounds and required approvals. The GUI must never infer completion from `Percent == 100`.

Preview is a separate authorized read adapter over a prepared-source lease. It uses opaque source IDs and bounded streaming, not a general local-file HTTP endpoint. Requests enforce current GUI session authorization, byte-range bounds, media type and lease lifetime. Future server authorization must map a principal to a workspace before invoking service commands; no user identity is inferred from an arbitrary source/cache ID.

CLI/TUI/GUI parity is evaluated by normalized plan digest and resulting state/provenance, not by identical screen layouts. Headless use requires no native dialog, browser window, GUI token or desktop event loop. Existing browser-cookie acquisition remains an explicitly selected adapter capability, with a clear failure if unavailable headlessly.

Long-running UI disconnects leave durable tasks intact while the workspace runs. Application exit stops execution safely; restarting any permitted interface recovers it under the same single-instance lock. Installer choices GUI/TUI/CLI must not create separate clip databases or caches for the same workspace.

## 33. Executable acceptance scenarios

Each scenario below becomes a named contract/integration test; UI-independent scenarios MUST first pass against in-memory ports and then against SQLite where transactions matter.

| ID | Given / action | Required evidence |
| --- | --- | --- |
| CORE-01 | Same source and 100 valid ranges admitted | One preparation; 100 ordered tasks; bounded worker count |
| CORE-02 | GUI and CLI encode equivalent intent | Same normalized request/plan digest, excluding transport-only IDs |
| CORE-03 | Identical admission replay after expiry/restart | Same original batch, no new source/task/upload |
| CORE-04 | Same key with changed range/destination | Conflict; database row counts unchanged |
| CORE-05 | Transaction fails halfway through admission | No partial batch, references, capacity reservations or key claim |
| CORE-06 | Two compatible admissions race | One elected preparation, both reference it safely |
| CORE-07 | One of two source consumers cancels | Other consumer finishes; source not removed |
| CORE-08 | Late render callback after retry | New epoch state and metrics remain unchanged |
| CORE-09 | Pause while FFmpeg renders | Pending pause reported; no next stage starts until resume |
| CORE-10 | Cancel races with publication intent | Deterministic reconciled done/canceled result, no hidden file |
| CORE-11 | Authoritative duration invalidates one range | All rendering blocked; invalid segment identified; no clamping |
| CORE-12 | Fast bounds change after approval | Prior approval rejected; no publication |
| CORE-13 | Unknown total/speed | Nullable progress/ETA, no fake zero or infinity |
| CORE-14 | Local source changes with same path | Candidate rejected; no source substitution or upload |
| CORE-15 | Disk fills during acquisition/render/publish | Writer reaped; attributable state retained; disk wait shown |
| CORE-16 | Ordinary downloads and clips compete | Combined permit/rate totals remain within configured limits |
| CORE-17 | Crash at every publication boundary | One asset, correct provenance, no overwrite or duplicate delivery |
| CORE-18 | Upload enqueued, dispatcher crashes before link | Replay links original upload; no duplicate object intent |
| CORE-19 | Empty explicit S3 selection, global AutoUpload true | Clip local-only; source private; zero clip upload intents |
| CORE-20 | Profile changes/vault locks after admission | Local clip completes; delivery blocks with precise recovery action |
| CORE-21 | Forget task history and replay admission key | Archived replay result; no duplicate media work |
| CORE-22 | Close races with Inspect/Admit/control/worker | Every admitted call joined before repository closes; race suite clean |
| CORE-23 | Cache cleanup races with preview/render | Leased source remains readable; no dangling deletion |
| CORE-24 | Missing/corrupt source candidate after restart | Revalidated recovery; never guessed ready by filename |
| MEDIA-01 | CFR and VFR marked frame fixtures | Exact selected frame IDs match half-open interval contract |
| MEDIA-02 | Audio offset/delay and separate tracks | Boundary and introduced sync error within section 6 thresholds |
| MEDIA-03 | Fast long-GOP/open-GOP samples | Expansion accepted; independent full decode; unsupported paths rejected |
| MEDIA-04 | Matching codec precise request | Encoding is still used; no opportunistic copy |
| MEDIA-05 | Unsupported HDR/timeline/codec | Typed unavailable capability; no degraded success |
| MIG-01 | Nonempty schema-5 workspace migration/reopen | Existing jobs/assets/playlists/vault/uploads retain identity and contents |
| MIG-02 | Migration interruption or newer-schema reader | Atomic failure or explicit refusal; no silent data loss |

Fixtures are generated locally with deterministic video frame IDs/audio impulses and require no network, user cookies or cloud keys. Optional live-provider smoke tests are separate and cannot replace these gates. Test invalid path characters, Unicode normalization/case collisions on supported filesystems, source-symlink swaps, subprocess hangs, observer panic and malformed probe JSON.

## 34. Implementation sequence with reviewable gates

The following expands section 22 and is the default implementation order. Each branch should remain reviewable, preserve existing contracts, and merge only after its gate passes. No release or installer work is implied.

| Step | Work and integration points | Required merge evidence |
| --- | --- | --- |
| A | Add pure domain values, canonicalization, policies, stable errors and fake service dependencies | Unit/property/fuzz tests for times, identity, transitions; architecture dependency test |
| B | Add clip repository ports, schema migration, atomic admission, replay tombstones and epoch fencing | In-memory/SQLite shared contract suite, concurrent admission and nonempty migration fixtures |
| C | Extract workspace resource ownership; adapt ordinary Queue without changing downloader contract | Existing queue/rate/cancel tests plus mixed clip/download resource tests under race detector |
| D | Implement local-source coordinator, accounted cache, references/leases and recovery | CORE-06/07/14/15/23/24; no media publication yet |
| E | Implement pinned-toolchain precise render and independent verification evidence for initial profile matrix | MEDIA-01/02/04/05 on macOS and Linux; fixture evidence recorded |
| F | Implement journaled publication, provenance links, library protections and restart reconciliation | Fault injection at every filesystem/DB boundary; CORE-10/17/21 |
| G | Wire ClipService lifecycle and minimal headless CLI execution | Complete local-source precise vertical slice without any UI import |
| H | Add URL preparation through existing authentication/source adapters and chapter normalization | Single acquisition, identity refresh, no source AutoUpload, bounded metadata/network tests |
| I | Add GUI/TUI range editors, review, progress and row actions | Same service contracts; equivalent normalized intent; theme/keyboard/regression smoke tests |
| J | Add fast-copy capabilities and bounds approval | MEDIA-03 and CORE-12, no hidden fallback |
| K | Add explicit delivery outbox, preview and manifest export | CORE-18/19/20/23; auth/read-range/privacy tests |
| L | Optional section acquisition | Parity with full-source evidence and measured transfer accounting |

Step G is an engineering milestone; it is not the complete end-user release. A first publicly described feature must state which of steps H–K it includes. Partial acquisition remains optional and must not delay a correct full-source implementation.

For every step, inspect the current baseline before modifying files. Required checks: relevant Go tests, race tests for changed concurrency, `go vet ./...`, existing architecture checks, public English-content checks, and existing download/playlist/library regression tests appropriate to the change. FFmpeg/ffprobe fixture suites run on supported macOS/Linux hosts with recorded toolchain versions. Hosted CI unavailability is recorded explicitly rather than reported as a pass.

## 35. Observability and operational limits

Expose counters/gauges through an application diagnostic snapshot: runnable/waiting tasks by reason; active resource grants; prepared/referenced/cache-cleanup bytes; acquisition/render/verify/publication durations; retries and contract violations; pending publication/delivery intents; recovery actions. Metrics use bounded labels (phase/reason), not URL/title/task ID as unbounded label values.

Persist concise per-attempt history, bounded to a configurable diagnostic retention policy; retain the latest failure and publication/provenance evidence independently. Redact URL credentials, signed query values, browser profile selectors and tokens before persistence. Truncate stderr/log messages by bytes without producing invalid UTF-8. Structured logs identify task/source/epoch so failures can be correlated without exposing private titles.

Runtime limits in section 5 are admission policy, configurable only within validated safe ranges. Defaults must be surfaced in capabilities/help. Listing ten thousand historical tasks cannot load every source manifest or hash every output; expensive revalidation happens on reuse/execution, not normal UI polling. Progress updates are coalesced to at most four visible updates per second per active entity; persistent phase transitions remain immediate, periodic metrics checkpoints at most once per second per entity. Terminal/error/control changes bypass progress coalescing.

## 36. Final decisions and implementation checklist

The following are settled v1 decisions, not questions to defer to a frontend developer:

- Business rules reside in independent domain/application Core; adapters supply facts and side effects.
- One finite source per batch; multiple independent outputs, no montage.
- Precise local/full-source processing is the correctness baseline; section downloading is optional later.
- Every source range is validated before any output starts; invalid authoritative bounds block the batch.
- Fast-copy requires explicit prepared-source-bound approval and never replaces precise mode silently.
- Normal downloads and clips use one workspace resource authority with typed owners.
- Durable admission is independent of frontend connection lifetime; execution still needs a running workspace owner.
- Immutable reviewed intent, separate control/progress revisions and attempt epochs prevent stale writes.
- Publication is journaled and reconciled; callbacks and successful subprocess exit never establish completion alone.
- Optional S3 is an explicit, durable, idempotent per-materialization delivery policy; empty means local-only.
- Existing downloader, whole-video publication, playlist identity and asset download IDs keep their meanings.
- Initial profile support is capability gated and proven by source-content fixtures; unsupported paths fail explicitly.
- Source caches, provenance and leases are workspace scoped and usable without a GUI or desktop session.
- Component auto-update, multi-user server, clip templates on subscriptions, subtitles and HDR are separate scopes.

Before calling the feature complete, attach an evidence checklist to its final integration PR covering CORE-01–24, MEDIA-01–05, MIG-01–02, supported profile/platform/toolchain matrix, all frontend parity tests, unchanged existing workflows, documented limitations, and the exact shipped scope. No unchecked recovery or accuracy gate may be replaced by a UI warning claiming that the result is verified.


## 37. Precise increment implementation status

The first implementation follows the narrower delivery option in section 2.2. [Operational documentation](../clips.md) is authoritative for available commands and current boundaries; this design retains normative requirements for later increments rather than presenting them as delivered.

Implemented: independent domain/application services and injected adapters; integer interval validation; manual/chapter plans; reviewed digest/idempotent admission; local snapshot or one full remote source per batch; precise video/audio encoding and decode/timeline/hash evidence; shared network/processing resources; separate durable clip records; publication reconciliation and provenance; explicit selected S3 delivery outbox; GUI/TUI/CLI batch use and explicit source-cache cleanup. Schema 6 is allocated by this implementation.

Not yet delivered: fast-copy/approval, section acquisition, preview, cross-batch source/materialization reuse, automatic retry/backoff, per-task pause, history compaction/tombstones, automatic cache TTL/budget and continuous staging reservation accounting. Source/cache operations currently use a sequential batch worker and entry-stage free-space checks. These limitations are explicit and are not replaced by unverified success claims. TUI manual ranges are implemented; source chapter selection is exposed through GUI and CLI.

Current persistence uses indexed batch rows with bounded ordered task arrays rather than the complete normalized table proposal of section 13. Domain/application CAS revisions and per-task attempt counters are implemented; distributed claims/ownership tokens are deferred with distributed workers. The single workspace owner plus inherited child locks prevents competing local workers. Adapter callbacks are closed and serialized at the boundary. Public service names and HTTP routes follow the operational documentation and source code; earlier snippets remain design sketches.

The complete-v1 definition of done in section 22 remains open until its remaining capabilities and fault/fixture matrix are implemented. No release, installer rebuild or server edition is included in this increment.


## Related companion extension

The proposed [Media Preview and Subtitles specification](media-companions-specification.md) defines poster/timeline/playback distinctions, selected caption discovery and retention, source leases, authenticated media reads, transcript-driven interval proposals and optional synchronized subtitle outputs. It is a separate, unimplemented extension; the precise increment status above remains unchanged.
