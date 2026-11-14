# Audiovisual Sync Conformer

## Goal

Audiovisual Sync Conformer is a self-hosted Go service for finishing
teams that need to measure and correct timing drift between a recorded audio
stream and an uncompressed picture stream. A take combines one 16-bit PCM WAV
file with one planar 4:2:0 Y4M file. The service detects the strongest paired
audio transient and picture flash, records an immutable sync analysis, and
produces a review bundle whose audio is aligned to picture timing.

The service is deliberately narrow. It does not edit creative timing, detect
general scene cuts, mix multiple recordings, or replace a non-linear editor. It
provides a deterministic first-pass conform and an auditable receipt that an
assistant editor can inspect before moving the material into a finishing
system.

## Users

- Assistant editors reviewing dual-system sound before editorial turnover.
- Finishing engineers validating camera and recorder clock drift.
- Review operators producing a bounded proxy bundle for a sync decision.

## Entities and invariants

### Media take

A `MediaTake` contains one immutable WAV payload, one immutable Y4M payload,
their media descriptors, a combined content digest, and an admitted timestamp.
The stored digest always describes the exact two payloads. A take identifier is
unique and may be retried only with the same name, audio bytes, and picture
bytes.

### Sync analysis

A `SyncAnalysis` belongs to one admitted take. It records:

- the detected audio transient time in milliseconds;
- the detected picture flash time in milliseconds;
- the signed offset, where a positive value means audio is late;
- a confidence value and a drift estimate in parts per million;
- the threshold policy used for the decision.

An analysis is immutable after it reaches `ready` or `failed`. A `ready`
analysis must have a finite offset and a positive event prominence. A failed
analysis stores one stable error code and never exposes a usable event pair.

### Conform job

A `ConformJob` belongs to one ready analysis. It corrects the audio by the
inverse of the measured offset while preserving picture frame order and Y4M
geometry. A conform may additionally request delivery of the picture at a
different constant frame rate. The conversion preserves the source playback
duration by re-timing frames (nearest-center selection, no interpolation and
no audio resampling); it never rewrites only the frame-rate label, which
would fast- or slow-play the material. Requests without a target frame rate
keep the historical behavior and re-emit the source rate and frame count. The
job commits a ZIP bundle containing:

- `audio-aligned.wav`;
- `video-conformed.y4m`;
- `conform-receipt.json`.

The receipt records separate source and output picture descriptors so the
admitted source picture and the actually delivered picture can always be
distinguished, including their digests. The completed receipt digest and byte
count always match the visible ZIP. Failed jobs never expose a bundle route.

## State machines

### Media take

`validating -> admitted`

Validation failures do not create a visible take. Repeating an identical
create request returns the admitted record. Reusing an identifier with
different bytes or a different name returns a conflict.

### Sync analysis

`queued -> detecting -> ready`

or

`queued -> detecting -> failed`

`ready` and `failed` are terminal. The service persists each state before
continuing, so a reader can distinguish an in-progress analysis from a
completed one.

### Conform job

`queued -> rendering -> committing -> completed`

or

`queued -> rendering -> failed`

or

`rendering -> committing -> failed`

`completed` and `failed` are terminal. The bundle and receipt are written in a
temporary directory and become visible only through one directory rename.

## Workflows

### 1. Admit a media take

Entry point: `POST /v1/takes`

1. Validate the identifier, name, size budgets, and strict base64 encoding.
2. Probe every WAV chunk and every Y4M frame before accepting the take.
3. Measure media descriptors and compute a combined digest.
4. Persist payloads and metadata under an exclusive take namespace.

Success returns the immutable take record. Failure returns a stable validation
or conflict code and leaves no visible take directory.

### 2. Detect audiovisual drift

Entry point: `POST /v1/takes/{take_id}/sync-analyses`

1. Load the admitted take and create a queued analysis.
2. Decode both media payloads again at the analysis boundary.
3. Detect a sharp audio onset and a picture luma flash.
4. Compare event times against duration and confidence limits.
5. Persist a ready or failed terminal record.

Success returns measured event times, signed offset, confidence, and drift.
Failure records a stable code such as `sync_detection_failed`.

### 3. Render a conformed review bundle

Entry point: `POST /v1/sync-analyses/{analysis_id}/conforms`

1. Require a ready analysis and enforce the requested correction ceiling.
2. Align audio samples by the inverse measured offset.
3. When a target frame rate is supplied, re-time the picture to that constant
   rate while preserving playback duration; otherwise re-emit the Y4M header
   and frames with the validated source geometry and rate.
4. Build the receipt with source and output picture descriptors, then the ZIP
   bundle and digests in a temporary directory.
5. Commit the directory atomically and complete the job.

Success returns a completed job whose receipt and bundle routes agree. Failure
returns a terminal job with a stable error code and no readable bundle.

## Modules

- `internal/httpapi`: public routing, JSON decoding, request limits, and error
  status mapping.
- `internal/service`: workflow orchestration, idempotency, lifecycle
  transitions, and failure propagation.
- `internal/domain`: identifiers, records, state rules, and stable errors.
- `internal/media`: WAV and Y4M probing, signal detection, audio alignment,
  duration-preserving frame-rate conversion, and deterministic media encoding.
- `internal/storage`: namespaced persistence, per-identifier locking, atomic
  files, and atomic directory publication.

The `admit-take` workflow crosses all five modules. The other two workflows
cross the HTTP boundary, service orchestration, media processing, domain rules,
and storage.

## Public interface

The service listens on `/healthz` and these resource routes:

- `POST /v1/takes`
- `GET /v1/takes/{take_id}`
- `POST /v1/takes/{take_id}/sync-analyses`
- `GET /v1/sync-analyses/{analysis_id}`
- `POST /v1/sync-analyses/{analysis_id}/conforms`
- `GET /v1/conforms/{conform_id}`
- `GET /v1/conforms/{conform_id}/bundle`

JSON errors use a stable envelope:

```json
{
  "error": {
    "code": "invalid_media",
    "message": "video decoder rejected frame payload"
  }
}
```

## Validation plan

The initial baseline intentionally defers automated tests. A later task stage
will add unit, integration, and HTTP verification for the production
interfaces below:

- WAV chunk bounds and Y4M frame-count agreement with declared geometry;
- onset and flash detection on calibration media with known event times;
- positive and negative audio corrections around the measured offset;
- constant frame-rate conversion preserving duration, clapper and tail flash
  timing, and unchanged audio speed, including rejection of a label-only rate
  rewrite and of conversions beyond the frame-count budget;
- terminal-state behavior for invalid and boundary requests;
- atomic visibility of take directories and conform bundles.

Until that stage, four bounded production workflow checks exercise the real
HTTP routes, storage layout, media transforms, and persisted terminal records.
They are not a substitute for the later test suite.

## Deferred testing boundary

Automated tests are intentionally omitted from this baseline. The later task
stage owns creation of the test harness, fixtures, race checks, and test
command. This repository ships only executable production workflow checks.
