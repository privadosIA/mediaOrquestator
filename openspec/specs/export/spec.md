# export Specification

## Purpose
This capability defines the **export** phase of the `images` output mode. When `output.mode` is `images`, the per-scene pipeline skips TTS, probe, clip, and concat, and instead executes render (× variants) followed by export. The export phase is responsible for:

- Placing each rendered PNG into a stable, predictable per-project directory layout.
- Writing a mandatory `manifest.json` that records every parameter needed to reproduce each image.
- Optionally building a contact-sheet PNG via `ffmpeg`'s `tile` filter.

The export phase emits a single `phase_start` / `phase_end` event pair (phase name `export`) via `internal/platform/events`. It introduces no new external dependencies beyond `ffmpeg`, which is already a required binary.

## Requirements

### Requirement: Final images land in a stable per-project layout

The export phase SHALL copy or move each scene's rendered PNG from the temporary/cache area to the project output directory using the following naming rules:

- When a scene has `variants == 1` (the default), the file SHALL be written to `out/<project>/<scene_id>.png`.
- When a scene has `variants > 1`, variant *N* (1-based) SHALL be written to `out/<project>/<scene_id>_v<N>.png`.

The export phase SHALL create all intermediate directories (`out/<project>/`) as needed before writing any file.

Before writing any file, the export phase SHALL resolve the full set of output paths for all scenes in the project and SHALL reject the run with a descriptive error (wrapping per the `fmt.Errorf("export: …: %w", err)` convention) if two or more resolved output paths collide. No file SHALL be overwritten or partially written when a collision is detected.

#### Scenario: Single variant naming

WHEN a project contains a scene with `scene_id: "intro"` and `variants: 1` (or the field is absent, defaulting to 1)
THEN the export phase SHALL write the rendered PNG to `out/<project>/intro.png`
AND the file SHALL be a valid PNG decodable by `ffmpeg` or `ffprobe`.

#### Scenario: Multi-variant naming

WHEN a project contains a scene with `scene_id: "hero"` and `variants: 3`
THEN the export phase SHALL write three PNGs to `out/<project>/hero_v1.png`, `out/<project>/hero_v2.png`, and `out/<project>/hero_v3.png`
AND no file named `out/<project>/hero.png` (without the `_vN` suffix) SHALL be produced for that scene.

#### Scenario: Collision rejected before any file is overwritten

WHEN two scenes in the same project resolve to the same output path (for example, scene `scene_id: "a"` with `variants: 1` producing `a.png` and scene `scene_id: "a"` with `variants: 2` producing `a_v1.png` and `a_v2.png` is fine, but a duplicate `scene_id` with the same variant count is not)
THEN the export phase SHALL emit a descriptive error identifying the colliding paths
AND no file in `out/<project>/` SHALL be created, overwritten, or truncated
AND the process SHALL exit with a non-zero status.

### Requirement: manifest.json is mandatory and makes every image reproducible

The export phase SHALL always write `out/<project>/manifest.json` in `images` mode, regardless of the number of scenes or variants. The manifest SHALL be valid JSON and SHALL be written atomically (write to a temporary file in the same directory, then rename) so that a concurrent reader never observes a partial file.

Each entry in the `images` array of the manifest SHALL contain the following fields:

| Field | Type | Source |
|---|---|---|
| `scene_id` | string | `scene.id` from `scenes.yaml` |
| `variant` | integer (0-based) | The variant index rendered |
| `effective_seed` | integer (32-bit) | The seed actually passed to `sd` (explicit `scene.seed + variant_index`, or the random seed generated at render time) |
| `prompt` | string | The final composed positive prompt: `defaults.subject_prompt` (if set) + `" "` + `scene.prompt` |
| `negative_prompt` | string | `scene.negative_prompt` (if set) + `" "` + `defaults.negative_prompt` (if set); empty string if neither is set |
| `model` | string | The resolved model path passed to `sd` |
| `resolution` | [integer, integer] | `[width, height]` as passed to `sd` |
| `sampler` | string | The sampler name passed to `sd` |
| `steps` | integer | The step count passed to `sd` |
| `file` | string | The relative path of the exported PNG (e.g. `out/<project>/intro_v1.png`) |

The top-level manifest object SHALL include:

| Field | Type | Source |
|---|---|---|
| `project` | string | The project name (directory name under `out/`) |
| `mode` | string | Always `"images"` |
| `generated_at` | string (RFC 3339) | UTC timestamp of the export run |
| `images` | array | One entry per exported PNG, ordered by scene order then variant index |

The manifest SHALL be the single source of truth for any downstream consumer (dashboard, CI, re-rendering) to reproduce an identical image: given the `effective_seed`, `prompt`, `negative_prompt`, `model`, `resolution`, `sampler`, and `steps` fields, a subsequent `sd` invocation SHALL produce a pixel-identical result.

#### Scenario: Manifest fields present for a 2-variant scene

WHEN a project has one scene (`scene_id: "hero"`, `variants: 2`, `seed: 100`, `prompt: "a red car"`, `defaults.subject_prompt: "cinematic"`, `defaults.negative_prompt: "blurry"`, `scene.negative_prompt: "text"`)
THEN `out/<project>/manifest.json` SHALL contain an `images` array of length 2
AND the first entry SHALL have `variant: 0`, `effective_seed: 100`, `prompt: "cinematic a red car"`, `negative_prompt: "text blurry"`
AND the second entry SHALL have `variant: 1`, `effective_seed: 101`, `prompt: "cinematic a red car"`, `negative_prompt: "text blurry"`
AND both entries SHALL include `model`, `resolution`, `sampler`, `steps`, and `file` fields with the values actually passed to `sd`.

#### Scenario: Random-seed run records the concrete seeds

WHEN a scene has no explicit `seed` field and `variants: 2`
THEN the export phase SHALL generate two random 32-bit seeds at render time
AND the manifest entries for that scene SHALL record those concrete seed values in `effective_seed`
AND re-running `sd` with the recorded `effective_seed` and all other manifest parameters SHALL reproduce the identical image.

#### Scenario: Manifest is what a later run or dashboard can use to re-render an identical image

WHEN a consumer reads `out/<project>/manifest.json` and extracts one entry
THEN that entry SHALL contain sufficient information (`effective_seed`, `prompt`, `negative_prompt`, `model`, `resolution`, `sampler`, `steps`) to invoke `sd` with identical parameters
AND the resulting PNG SHALL be pixel-identical to the file at the path given by the entry's `file` field.

### Requirement: Contact sheet is optional and built with ffmpeg tile

When `output.contact_sheet` is `true` in `scenes.yaml`, the export phase SHALL build `out/<project>/contact_sheet.png` from all exported PNGs in the project using `ffmpeg`'s `tile` filter. The invocation SHALL use the existing `CommandRunner`/`Runner` interface pattern (no direct `os/exec` call) and SHALL introduce no new external dependencies beyond `ffmpeg`, which is already a required binary.

When `output.contact_sheet` is `false` or the field is absent, the export phase SHALL NOT produce a contact sheet and SHALL NOT invoke `ffmpeg` for this purpose.

A failure in the contact-sheet `ffmpeg` invocation (non-zero exit, missing binary, I/O error) SHALL NOT invalidate or delete already-exported PNGs or the `manifest.json`. The export phase SHALL report the failure as an error (wrapping per the `fmt.Errorf("export: contact sheet: %w", err)` convention) and the process SHALL exit with a non-zero status, but all previously written artefacts SHALL remain intact on disk.

#### Scenario: Enabled produces the sheet

WHEN `output.contact_sheet` is `true` and the project has three exported PNGs
THEN the export phase SHALL invoke `ffmpeg` with the `tile` filter to compose `

### Requirement: Music audio lands in the project output directory

In music mode the `export` phase SHALL copy each synthesized audio file to `out/<project>/` with the names defined by the `music` capability and SHALL write `out/<project>/manifest.json` in a music shape that declares `mode: "music"` and one entry per exported variant. The manifest SHALL be written atomically. With `--only`, the files of other tracks SHALL stay untouched and the manifest SHALL keep the entries of the other tracks that the previous manifest held, provided the track is still in the project and its audio file still exists, together with the new entries of the processed track, in project order. A run without `--only` SHALL write the manifest from the tracks it processed.

#### Scenario: Export of a project
- **WHEN** a music project with tracks `a` and `b` finishes
- **THEN** the audio of both is in `out/<project>/` and the manifest has an entry for each

#### Scenario: Manifest declares its mode
- **WHEN** a consumer reads the manifest of a music run
- **THEN** it can tell from `mode` that the entries describe songs, not images

#### Scenario: Failed export keeps earlier files
- **WHEN** writing the manifest fails after the audio was copied
- **THEN** the copied audio remains on disk and the run exits non-zero

#### Scenario: Regenerating one track keeps the others
- **WHEN** a project with tracks `a` and `b` was exported and the user runs only `b`
- **THEN** the manifest has the entries of `a` as before and the new entries of `b`, in project order

#### Scenario: Entries of tracks that are gone are not kept
- **WHEN** a run restricted to `b` finds the previous entry of `a` and `a` was removed from the project, or its audio file no longer exists
- **THEN** that entry is not in the new manifest

#### Scenario: An unreadable previous manifest is ignored
- **WHEN** a run restricted to one track finds a previous manifest that cannot be read
- **THEN** the run succeeds and the manifest has the entries of the processed track

### Requirement: The manifest records task and source

A music manifest entry of a track with a task SHALL carry `task` and `source`. Entries of new songs SHALL NOT carry them.

#### Scenario: Cover entry
- **WHEN** a cover track is exported
- **THEN** its entry has `task: cover` and its `source`

#### Scenario: New song entry
- **WHEN** a track without task is exported
- **THEN** its entry has neither field

### Requirement: The manifest records the reference

A music manifest entry of a track with a reference SHALL carry `reference`; others SHALL NOT.

#### Scenario: Entry
- **WHEN** a track with a reference is exported
- **THEN** its entry has `reference` with the file name

### Requirement: The images manifest records the starting picture

A manifest entry of a scene with `init_image` SHALL carry `init_image` and `strength`; others SHALL NOT.

#### Scenario: Entry
- **WHEN** such a scene is exported
- **THEN** its entry has both fields

### Requirement: The music manifest lists the mixes

The music `manifest.json` SHALL carry, when the project has mixes, a `mixes` list with one entry per mix: `id`, `file` (the exported path), `audio_seconds`, and its `clips` as rendered (the source — `track` and `variant`, or `file` — and `start`, `gain`, `mute`, `fade_in`, `fade_out`, `trim_start`, `trim_end`). A manifest of a project without mixes SHALL NOT contain the field. A run restricted to one track or one mix SHALL keep the other entries of the previous manifest, like it does for tracks. Mix files SHALL take part in the check that exported names do not collide.

#### Scenario: Manifest with a mix
- **WHEN** a mix is rendered
- **THEN** `mixes` holds its id, file, length and clips

#### Scenario: No mixes
- **WHEN** the project has none
- **THEN** the manifest has no `mixes` key

#### Scenario: A mix file name clashes with a track
- **WHEN** a mix would be exported to the same path as a track variant
- **THEN** the run is rejected with the collision named
