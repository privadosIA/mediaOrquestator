# song-analysis Specification

## Purpose
Defines how a song or a video kept in a project's sources is read on this machine: its lyrics and language (Whisper, on the CPU, with its safeguards against looping and invented text), its tempo and key (estimated by the server and checked against songs whose values are known) and, for a video, its picture, so that the music studio can propose a track from it.

## Requirements

### Requirement: A source of the project can be an audio or a video

The analysis of a song SHALL take a source that is already in the project's `sources/`: an `.mp3` or `.wav`, or a video (`.mp4`, `.mov`, `.mkv`, `.webm`, `.avi`) that has been uploaded as a video and is kept as it is. For a video the server SHALL extract its audio to a temporary file with ffmpeg for the analysis and remove it afterwards; it SHALL NOT create a source from it unless the user asks for one. A video with no audio track SHALL be `422`. An audio source SHALL work exactly the same way, with no picture description.

#### Scenario: Only an mp3
- **WHEN** the source is an `.mp3`
- **THEN** the lyrics and the measures are produced from it and the answer has no picture description

#### Scenario: A video
- **WHEN** the source is a kept video
- **THEN** its audio is extracted to a temporary file, analysed, and the file is removed; the video stays in `sources/` as it was

#### Scenario: A video with no sound
- **WHEN** the video has no audio track
- **THEN** the answer is `422` saying so

### Requirement: An audio source can be transcribed to lyrics

`POST /api/projects/{id}/transcribe` `{"source", "language"?}` SHALL read the source's audio with `whisper-cli` and answer `200 {language, language_detected, text, segments: [{start, end, text}], dropped, model}`. `language` is an ISO 639-1 code; without it the language SHALL be detected first from the audio and reported with `language_detected: true`. The transcription SHALL run on this machine and SHALL NOT send the audio anywhere. The source SHALL only be read. `400` for a bad project or source or a file that is not audio or video, `404` for an unknown project or source, `409` while a run is queued or active, `422` when the audio cannot be read, `503` when `whisper-cli` or its model is missing (the message says how to install them) or the memory is short.

#### Scenario: A sung source
- **WHEN** a source of a song in Spanish is transcribed without `language`
- **THEN** the answer has `language: "es"`, `language_detected: true`, the segments in playing order with their seconds and `text` as the segments joined by line breaks

#### Scenario: The language is given
- **WHEN** the request names `language: "en"`
- **THEN** no detection is made and `language_detected` is false

#### Scenario: Whisper is not installed
- **WHEN** `whisper-cli` or the model is missing
- **THEN** the answer is `503` with the command that installs it and nothing is written

#### Scenario: A run is in progress
- **WHEN** a run is queued or active
- **THEN** the answer is `409` and whisper is not started

### Requirement: The transcription does not carry the previous text into the next window

`whisper-cli` SHALL be called with the previous-text context turned off (`-mc 0`), with the language fixed (given or detected, never left to each window), without translation and with the JSON output. The voice-activity filter SHALL NOT be used: it is made for speech and, measured on a sung line, it cut the first seven seconds that the call without it heard whole. A song repeats lines, and carrying the text over makes the model repeat itself or invent; a published benchmark of lyrics transcription found this setting mattered more than the model.

#### Scenario: The flags of the call
- **WHEN** a source is transcribed
- **THEN** the process is started with `-mc 0`, the language and `-oj`, and without `--vad`

### Requirement: Looping and empty segments are dropped, and the answer says how many

The segments SHALL be cleaned before they are returned: empty ones, ones with no letters, and any run of more than 3 identical consecutive segments beyond the third (the loop a model falls into over music with no singing). `dropped` SHALL be the number removed. When nothing is left the answer SHALL be `200` with an empty `text` and `segments`, so the caller treats the source as having no words; the server SHALL NOT invent text.

#### Scenario: A loop
- **WHEN** the model answers the same line 12 times in a row
- **THEN** the answer keeps 3 of them and `dropped` is 9

#### Scenario: An instrumental source
- **WHEN** the audio has no singing and the model returns nothing usable
- **THEN** the answer is `200` with `text: ""`, `segments: []` and no error

### Requirement: Transcribing the separated voice is offered, not required

The voice stem that the `extract` task already produces (`vocals`) SHALL be transcribable like any other source. Separating first SHALL be a choice of the caller and never done by the transcription itself, because it is a generation on the GPU with a model that is not the turbo one. The studio SHALL offer it as an option, off at first, and SHALL say that it takes a generation and that in the test made with the project's own songs it changed the result little.

#### Scenario: The stem is a source
- **WHEN** the voice of a song has been separated and the result is copied to `sources/`
- **THEN** `POST /api/projects/{id}/transcribe` accepts it like any other source

### Requirement: The tempo and the key of an audio are measured, as estimates

`POST /api/projects/{id}/measure` `{"source"}` SHALL answer `200 {duration, bpm, bpm_alternatives, bpm_confidence, keyscale, key_confidence}` computed by the server itself from the audio, on the CPU, with no model and no new program beyond ffmpeg. `bpm` is a number from 60 to 200 with the half and double tempo in `bpm_alternatives` when they fall in that range; `keyscale` is written as the music tracks write it (`D minor`, `F# major`); both confidences are from 0 to 1. A source without a clear pulse SHALL answer `bpm: null`, and one without a clear key `keyscale: null`, never a guess. The values are estimates and the documentation SHALL say how often they matched in the songs they were checked against. Time signature SHALL NOT be measured. `400`, `404`, `422` and `503` as for the transcription; it does not wait for runs (it uses no GPU and little memory).

#### Scenario: A song with a pulse
- **WHEN** a song of 120 bpm in D minor is measured
- **THEN** the answer has a `bpm` near 120 (or its half or double listed in `bpm_alternatives`) and a `keyscale` with a confidence

#### Scenario: No clear pulse
- **WHEN** the audio is a held ambient chord
- **THEN** `bpm` is `null`

#### Scenario: The source is a video
- **WHEN** the source is a kept video
- **THEN** its audio is measured and the video is not modified

### Requirement: The measure is checked against songs whose tempo and key are known

Before it is offered, the estimator SHALL be run on at least 12 songs of the project's own library whose bpm and key were requested when they were generated, and the result (how many bpm within 4 % of the requested one or at its half or double, how many keys exact or relative) SHALL be written in `docs/MUSIC_MODE.md` with the number of songs. If the hit rate of the bpm is below 70 %, the studio SHALL show the values as approximate and never fill the fields by itself.

#### Scenario: The check is documented
- **WHEN** the change is archived
- **THEN** the documentation states the number of songs checked and the hit rates measured

### Requirement: A kept video can also be described by its picture

The picture of a kept video SHALL be described with the endpoint that already describes a clip, `POST /api/describe` `{"project", "source"}`, with no new route and none of its safeguards changed: key frames, the local vision model or the routed provider, a refusal reported and never replaced, and a person who is or looks under 18, or whose age cannot be confirmed, ending the request with `describe_minor` and the video deleted. An audio SHALL be `400`.

#### Scenario: A music video
- **WHEN** a kept video is described
- **THEN** the answer carries a prompt of what is seen and the model that wrote it

#### Scenario: A minor in a frame
- **WHEN** a frame shows a person who looks under 18
- **THEN** the answer is `422 describe_minor`, no text is returned and the video is deleted

### Requirement: A hidden project is not analysed

The routes live under `/api/projects/{id}/` and SHALL be classified in the content gate like the other project routes: a project the server's content mode hides answers `404` with the body of an unknown project.

#### Scenario: Censored server
- **WHEN** the server is censored and the project is marked `adult: true`
- **THEN** the answer is the same `404` as for a project that does not exist

### Requirement: The transcription needs free memory and says so

Before starting, the server SHALL check the memory available against the model's size (the file plus a margin) and answer `503` with the amount missing when it does not fit, never starting a process that would exhaust the machine. The process SHALL be limited in time (10 minutes) and closing the request SHALL cancel it.

#### Scenario: Memory is short
- **WHEN** the available memory is below what the model needs
- **THEN** the answer is `503` that says how much memory is missing and no process starts

#### Scenario: Cancelled
- **WHEN** the client closes the request while it is transcribing
- **THEN** the process is stopped and nothing is left in the project

### Requirement: The server listens to a recording of a song and says what kind it is

`POST /api/assist/listen` SHALL take a recording of a song that is playing (4 to 60 seconds, up to 10 MB), measure its tempo and key and how it sounds (timbre, bass, texture, dynamics, density of the rhythm and loudness, in plain words), read a few words of its lyrics and answer the language, whether no words were recognised (fewer than four), the tempo and the key only when their confidence is at least 0.25, how it sounds, a style description in English and what it is about, and `request`, a sentence that asks for a new original song of that kind. A recording with no recognised words SHALL NOT be declared instrumental and the style SHALL say nothing about a voice. When the assistant cannot tell the genre it SHALL say so, the answer SHALL mark it (`genre_unknown`) with a warning, and the style SHALL then only describe the sound. The answer SHALL NOT contain the lyrics that were heard, and neither the recording nor its text SHALL be kept or logged. It SHALL wait for runs like the assistant, answer `422` for an unreadable or too short recording, `413` over the limits and `503` without Whisper, ffmpeg, memory or the assistant.

#### Scenario: a song with words

- **WHEN** a recording of 20 seconds of a sung techno song is sent
- **THEN** the answer has its language, tempo, key, sound, style and theme and a `request`, and none of its words

#### Scenario: a song without words

- **WHEN** the recording has fewer than four recognised words
- **THEN** the answer says that no words were recognised, has no theme and does not call the song instrumental

#### Scenario: a genre that cannot be told

- **WHEN** the assistant answers that the genre is unknown
- **THEN** the answer is marked `genre_unknown`, carries a warning and its style only describes the sound

#### Scenario: too short

- **WHEN** the recording lasts less than 4 seconds
- **THEN** the answer is `422` saying that at least 4 seconds are needed

### Requirement: The recognition methods are listed and chosen

`GET /api/music/recognizers` SHALL answer `{recognizers: [{id, available, reason?, detail?}]}` for `whisper` (always available), `clap` (available when its service is running; otherwise the engine's `reason` and `detail`) and `muq` (MuQ-MuLan, run with the `diffrhythm2 classify` executable published in `bin/services/diffrhythm2`: available when that executable and `mulan_audio.safetensors` exist; otherwise unavailable with `no_service` or `no_weights` and a `detail`). It SHALL start nothing.

#### Scenario: CLAP stopped

- **WHEN** the CLAP service is stopped
- **THEN** `clap` is unavailable with `not_running` and `whisper` is available

### Requirement: A source can be heard with LAION-CLAP

`POST /api/projects/{id}/recognize {source, method: "clap"}` SHALL send the first 30 seconds of the audio or kept video to the CLAP service and answer `{method, genre, mood, instrument, tags}`, the best 3 genres, 2 moods and 4 instruments of the music tag vocabulary with their scores, and `tags` with all of them in that order. It SHALL NOT start the service: when it is not running the answer is `503` with the engine's reason in `error.reason`. `method: "muq"` SHALL read the first 60 seconds with MuQ-MuLan (`diffrhythm2 classify` on the CPU) and answer the same shape; when the executable or `mulan_audio.safetensors` is missing it SHALL be `503` with `no_service` or `no_weights` in `error.reason`. `whisper` or any other value SHALL be `400`.

#### Scenario: A song

- **WHEN** CLAP is running and a source is recognised with `clap`
- **THEN** the answer has the tags in the order genre, mood, instrument

#### Scenario: MuQ

- **WHEN** the method is `muq` and the `diffrhythm2` executable and `mulan_audio.safetensors` exist
- **THEN** the answer has the tags of genre, mood and instrument read by MuQ-MuLan

#### Scenario: MuQ without weights

- **WHEN** the method is `muq` and the converted audio weights are missing
- **THEN** the answer is `503` with `no_weights` in `error.reason`

### Requirement: A recorded song can be heard with LAION-CLAP

`POST /api/assist/listen?method=clap` SHALL, besides what it already does, read the recording's sound with CLAP and give its tags to the assistant as a hint, returning them in `heard`. With CLAP stopped the answer SHALL be `503` with the reason and the assistant SHALL NOT run. `method=muq` SHALL do the same with MuQ-MuLan (`503` with the reason when it is not set up), another value `400`, and no `method` or `whisper` the existing behaviour with no `heard`.

#### Scenario: With CLAP

- **WHEN** a recording is sent with `method=clap` and CLAP runs
- **THEN** `heard` has the tags and the assistant's input contains them

#### Scenario: CLAP stopped

- **WHEN** CLAP is stopped
- **THEN** the answer is `503` with `not_running` and the assistant is not asked

### Requirement: The voice separator is a program of the machine

The server SHALL be able to separate the voice of a song on the CPU with a demucs.cpp binary (`demucs`, looked up first in `bin/` and then in the `PATH`) and a Demucs 4-stem model in `models/demucs/` (`htdemucs` preferred when more than one is there), the way it uses `whisper-cli` and its model: no service, no network, no GPU. Separating SHALL produce the voice stem and discard the other stems, SHALL be limited in time (15 minutes) and cancelled when the request closes, SHALL check before starting that the model fits in the free memory (its size times a factor plus a margin, both measured once and written as constants) and SHALL NOT run while a run is queued or active. When the binary or the model is missing the caller SHALL be told so with the command that installs them (`bash scripts/get-demucs.sh`). The separator SHALL be optional: its absence SHALL never block a project of any mode. The existing separation with the `extract` task of ACE-Step is a different thing (a generation on the GPU) and is unchanged.

#### Scenario: Installed
- **WHEN** `bin/demucs` and `models/demucs/ggml-model-htdemucs-4s-f16.bin` exist and the memory suffices
- **THEN** a song is separated on the CPU and the voice stem is returned

#### Scenario: Not installed
- **WHEN** no `demucs` binary or no model is found
- **THEN** the caller is told what is missing and the command that installs it, and nothing runs

#### Scenario: A run in progress
- **WHEN** a run is queued or active
- **THEN** separation is not started

#### Scenario: Memory is short
- **WHEN** the free memory is below what the model needs
- **THEN** separation is refused with how much is missing
