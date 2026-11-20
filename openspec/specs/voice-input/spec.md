# voice-input Specification

## Purpose
TBD - created by archiving change add-voice-dictation. Update Purpose after archive.

## Requirements

### Requirement: A spoken sentence can be transcribed on the server

The server SHALL offer an endpoint that receives a short recording as the browser makes it (WebM/Opus, MP4/AAC, Ogg, WAV or MP3), transcribes it with the locally installed Whisper on the CPU and answers the text, the language it was spoken in and its duration. The recording SHALL NOT be kept after the answer and SHALL NOT leave the machine. An optional language (an ISO 639-1 code) SHALL fix the language; without it Whisper SHALL detect it. A recording without words SHALL answer success with an empty text. The text SHALL be returned as spoken: the cleaning that song transcription applies (dropping repeated lines and note-only segments) SHALL NOT be applied.

#### Scenario: dictating a request

- **WHEN** the phone sends a 6-second recording of "hazme un vídeo de un gato en una azotea al atardecer"
- **THEN** the answer carries that text, `es` as the language and the duration, and no file of the recording remains on the server

#### Scenario: a silent recording

- **WHEN** the recording has no speech
- **THEN** the answer is a success with an empty text

#### Scenario: the language is fixed

- **WHEN** the request names `en`
- **THEN** Whisper reads the recording as English and the answer says `en`

### Requirement: A dictation has limits and does not wait for runs

A recording SHALL be refused when it is longer than 60 seconds or larger than 5 MB, or when it cannot be read as audio, with a message saying which. A dictation SHALL NOT be refused because a run is queued or active, but SHALL be refused, saying how much memory is missing, when the available memory is not enough for the Whisper model. When `whisper-cli`, its model or ffmpeg is missing the endpoint SHALL answer that the voice reader is not installed and how to install it.

#### Scenario: dictating while a video is being generated

- **WHEN** a run is active and the user dictates a new request and the memory is enough
- **THEN** the dictation is transcribed normally

#### Scenario: too long

- **WHEN** a recording lasts 90 seconds
- **THEN** it is refused with a message saying the limit is 60 seconds

#### Scenario: not installed

- **WHEN** the Whisper model is not installed
- **THEN** the answer says the voice reader is not installed and names the script that installs it

### Requirement: Serving the dashboard over HTTPS is documented

The documentation SHALL explain how to serve the dashboard over HTTPS to a Tailscale network with `tailscale serve`, which gives a valid certificate without exposing the server to the internet, because browsers only give a page access to the microphone on a secure origin. The server SHALL keep requiring its token on that address.

#### Scenario: following the documentation

- **WHEN** the owner runs the documented `tailscale serve` command
- **THEN** the dashboard is reachable at an `https://` address of the tailnet and API calls still need the token
