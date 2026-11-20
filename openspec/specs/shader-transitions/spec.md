# shader-transitions Specification

## Purpose
TBD - created by archiving change add-shader-transitions. Update Purpose after archive.

## Requirements

### Requirement: A join can be a shader transition

A scene's `transition` (and the project's default) SHALL accept `shader:<name>` for each of the 14 shaders of HyperFrames' shader transitions, and refuse any other `shader:` name. The join SHALL show the shader mixing the end of the previous scene with the start of this one over the join's `transition_seconds`, and SHALL crossfade the sound over the same time.

#### Scenario: A shader join
- **WHEN** two scenes of 3 s are joined with `transition: shader:cross-warp-morph` and `transition_seconds: 0.5`
- **THEN** the video lasts 5.5 s, its picture at the start of the join is the first scene's and at its end the second's

#### Scenario: An unknown shader
- **WHEN** a scene has `transition: shader:nope`
- **THEN** the configuration is refused naming the field

### Requirement: A shader transition that cannot be drawn is a crossfade

When Chrome is missing or the transition fails, the join SHALL be a crossfade of the same length and a warning SHALL name the scene.

#### Scenario: No browser
- **WHEN** a join asks for a shader transition and there is no Chrome
- **THEN** the video is made with a crossfade there and the run warns
