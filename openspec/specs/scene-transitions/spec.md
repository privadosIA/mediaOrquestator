# scene-transitions Specification

## Purpose
TBD - created by archiving change add-scene-transitions. Update Purpose after archive.

## Requirements

### Requirement: Scenes can be joined with a crossfade

A video project SHALL accept `defaults.transition_seconds` and a scene SHALL accept `transition_seconds`, in seconds from 0 to 2. The join between two scenes SHALL be a crossfade of the length given by the later scene's `transition_seconds`, or by the project's when the scene has none; 0 (or absence of both) SHALL be a hard cut. The first scene's value SHALL be ignored. The validation SHALL reject a value outside the range, and either field in a project that is not video.

#### Scenario: Project option
- **WHEN** `defaults.transition_seconds` is 0.4 and no scene has its own
- **THEN** every join between two scenes is a 0.4 s crossfade

#### Scenario: Scene override
- **WHEN** the project fades 0.4 s and one scene has `transition_seconds: 0`
- **THEN** the join into that scene is a cut and the others still fade

#### Scenario: A longer fade for one scene
- **WHEN** one scene has `transition_seconds: 1`
- **THEN** only the join into that scene is a 1 s crossfade

#### Scenario: Out of range
- **WHEN** a value is -1 or 3
- **THEN** the validation names the field

### Requirement: A fade overlaps the scenes and crossfades the sound

During a crossfade the end of one scene and the start of the next SHALL overlap, so the final video SHALL be shorter by the sum of the fades, and the audio of the two scenes SHALL be crossfaded over the same time. A fade longer than half of the shorter scene it joins SHALL be shortened to that half and a warning event SHALL name the scene. When no join fades, the scenes SHALL be joined as the assemble capability says (the result encoded once, the index first).

#### Scenario: Total length
- **WHEN** three scenes of 2 s are joined with 0.5 s fades
- **THEN** the video lasts 5 s

#### Scenario: Too long for the scene
- **WHEN** a 1 s scene is asked for a 1.5 s fade
- **THEN** the fade lasts 0.5 s and a warning names the scene

#### Scenario: No fades
- **WHEN** no scene and not the project sets a fade
- **THEN** the join uses the concat demuxer and encodes the result once

### Requirement: The editor sets the transitions

The defaults panel SHALL have the project's fade, and the scene form the scene's own with the project's value as the empty default.

#### Scenario: Set both
- **WHEN** the user sets 0.4 in the project and 0 in a scene
- **THEN** the file has `defaults.transition_seconds: 0.4` and `transition_seconds: 0` in that scene

### Requirement: The kind of a transition can be chosen

A project and each scene SHALL be able to name the kind of transition into a scene from `fade`, `fadeblack`, `fadewhite`, `dissolve`, `wipeleft`, `wiperight`, `wipeup`, `wipedown`, `slideleft`, `slideright`, `slideup`, `slidedown`, `circleopen`, `circleclose`, `radial` and `pixelize`. The scene's kind SHALL win over the project's, and `fade` SHALL be used when neither is set. The kind SHALL only matter where the join has a fade length above 0. An unknown kind SHALL fail validation, and a kind in a project that is not a video SHALL be rejected. The editor SHALL offer the kinds in the project defaults and in each scene.

#### Scenario: Scene overrides the project
- **WHEN** the project says `fadeblack` and a scene says `slideleft`
- **THEN** the join into that scene uses `slideleft` and the others `fadeblack`

#### Scenario: Default
- **WHEN** no kind is set
- **THEN** the join is the crossfade

#### Scenario: Unknown kind
- **WHEN** a scene names `spin`
- **THEN** validation fails and lists the kinds

#### Scenario: Every kind joins
- **WHEN** two clips are joined with each kind
- **THEN** ffmpeg produces the video
