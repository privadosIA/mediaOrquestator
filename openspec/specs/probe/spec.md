# probe Specification

## Purpose
Defines how a scene's synthesized audio duration is measured via ffprobe, so the render/clip phases can time that scene's video correctly.

## Requirements

### Requirement: Duration reports an audio file's length in seconds
`Duration` SHALL invoke ffprobe to read the `format=duration` of the file at `path` and SHALL return it as a `time.Duration`.

#### Scenario: valid ffprobe output
- **WHEN** ffprobe reports `format.duration` as `"3.500000"`
- **THEN** `Duration` returns `3.5` seconds

### Requirement: Duration propagates runner and parse errors
`Duration` SHALL return an error, without a usable duration, when the Runner fails or when ffprobe's output cannot be parsed.

#### Scenario: runner error
- **WHEN** the CommandRunner returns an error
- **THEN** `Duration` returns that error

#### Scenario: invalid JSON
- **WHEN** ffprobe's stdout is not valid JSON
- **THEN** `Duration` returns a non-nil error
