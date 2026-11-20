# tts Specification

## Purpose
Defines how a scene's narration text becomes a `.wav` audio file by driving the Piper TTS binary.

## Requirements

### Requirement: Synthesize translates speed into Piper's length-scale
`Synthesize` SHALL invoke Piper with `--length-scale` set to the reciprocal of the scene's speed (`1 / speed`).

#### Scenario: speed 2.0 halves length-scale
- **WHEN** `Synthesize` is called with `speed = 2.0`
- **THEN** Piper is invoked with `--length-scale 0.5`

### Requirement: Synthesize rejects non-positive speed without invoking Piper
`Synthesize` SHALL return an error for `speed <= 0` without invoking the Runner.

#### Scenario: zero or negative speed
- **WHEN** `speed` is `0` or negative
- **THEN** `Synthesize` returns a non-nil error
- **AND** the Runner is never invoked

### Requirement: Synthesize feeds ttsText over stdin and writes Piper's output to outPath
`Synthesize` SHALL pipe `ttsText` to Piper's stdin and SHALL write the WAV audio Piper emits on its stdout to the file at `outPath`. It SHALL request that stdout form from Piper (`--output_file -`) and SHALL NOT pass `outPath`, or any other file path, to Piper as an output argument. If Piper exits successfully but emitted no bytes, `Synthesize` SHALL remove `outPath` and return an error saying no audio was produced.

#### Scenario: successful synthesis
- **WHEN** `Synthesize` succeeds
- **THEN** the Runner received `ttsText` as stdin
- **AND** `outPath` contains the bytes Piper wrote to stdout
- **AND** the Piper arguments contain `--output_file -` and no path to `outPath`

#### Scenario: Piper emits nothing
- **WHEN** the Runner succeeds but returns no stdout bytes
- **THEN** `Synthesize` returns a non-nil error
- **AND** no file remains at `outPath`

### Requirement: Synthesize propagates Runner errors unchanged
`Synthesize` SHALL return the Runner's error unchanged when the Runner fails.

#### Scenario: Piper fails
- **WHEN** the Runner returns an error
- **THEN** `Synthesize` returns that same error

### Requirement: Synthesize selects the voice with --model
`Synthesize` SHALL receive the voice model path and SHALL invoke Piper with `--model <voice>`, passing the path exactly as given (it is resolved by Piper against its working directory). It SHALL return an error without invoking the Runner when the voice is empty.

#### Scenario: voice passed through
- **WHEN** `Synthesize` is called with voice `voices/en_US-lessac-medium.onnx`
- **THEN** Piper is invoked with `--model voices/en_US-lessac-medium.onnx`

#### Scenario: empty voice
- **WHEN** the voice is the empty string
- **THEN** `Synthesize` returns a non-nil error and the Runner is never invoked

### Requirement: Narration works when the pipeline runs under a project directory
When the pipeline runs with a project directory, narrated scenes SHALL be synthesized with the voice path reaching Piper project-relative (as `defaults.model` reaches `sd`), and the WAV SHALL be created at the project-resolved path, so that subprocess arguments never contain the project directory twice.

#### Scenario: narrated scene under a project directory
- **WHEN** a narrated scene runs with project directory `projects/demo`
- **THEN** Piper's working directory is `projects/demo`, its `--model` argument is project-relative, and the WAV exists at `projects/demo/tmp/<scene>.wav`
