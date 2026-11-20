# artifact-access Specification

## Purpose
Defines read-only HTTP access to a project's pipeline artifacts — per-scene intermediates, exported images, manifests and the final video — strictly confined to that project's directories.

## Requirements

### Requirement: Project artifacts are served read-only

The server SHALL serve, for a given project: per-scene intermediates from its `tmp/` (`.wav`, `.png`, `.mp4`), exported PNGs, `manifest.json` and contact sheet from its `out/`, and the final video. Responses SHALL carry a media type appropriate for in-browser playback/display, and SHALL support range requests so media seeking works. The API SHALL offer no endpoint that writes or deletes artifact files.

#### Scenario: playing an intermediate clip

- **WHEN** a scene's intermediate `.mp4` is requested
- **THEN** it is returned with a video media type and range-request support

#### Scenario: images-mode manifest

- **WHEN** a project's `manifest.json` is requested after an images-mode run
- **THEN** its JSON content is returned as written by the export phase

### Requirement: Artifact paths are confined to the project

Artifact requests SHALL only ever resolve to files inside the requesting project's own directories. A request whose resolved path escapes them (path traversal, absolute paths, symlinks pointing outside) SHALL be rejected without revealing filesystem details.

#### Scenario: traversal is rejected

- **WHEN** an artifact request contains a path that resolves outside the project directory
- **THEN** the request is rejected and no file content is returned

### Requirement: Missing artifacts are distinguishable from other errors

Requesting an artifact that does not exist (not yet produced, or cleaned because `tmp/` was not kept) SHALL return a not-found response distinct from validation or server errors, so the UI can show "not produced yet" states.

#### Scenario: artifact not yet produced

- **WHEN** a scene's `.wav` is requested before any run produced it
- **THEN** the response is a not-found error naming the missing artifact

### Requirement: Audio artifacts are served for playback

Project audio files (`.mp3`, `.wav`) SHALL be served read-only with the correct media type and support for byte-range requests, so that a browser can seek while playing. The same confinement and missing-artifact rules as other artifacts SHALL apply.

#### Scenario: Range request
- **WHEN** a client requests part of an exported MP3 with a `Range` header
- **THEN** the server answers `206` with only that part

#### Scenario: Path outside the project
- **WHEN** the requested path escapes the project directory
- **THEN** the request is rejected as for any other artifact

### Requirement: Artifacts are revalidated by the browser

Every artifact answer SHALL carry `Cache-Control: no-cache` and keep its `Last-Modified`, so a browser revalidates it on each use (a `304` when unchanged) instead of reusing a heuristic copy of an earlier run. A file rewritten in place by a later run, such as a regenerated song or an updated `manifest.json`, SHALL therefore be seen as soon as it is read again.

#### Scenario: Revalidation headers
- **WHEN** an artifact is requested
- **THEN** the answer has `Cache-Control: no-cache` and a `Last-Modified` header

#### Scenario: A rewritten manifest
- **WHEN** a run rewrites `manifest.json` and the page reads it again
- **THEN** the page receives the new content, not the copy it read before the run

