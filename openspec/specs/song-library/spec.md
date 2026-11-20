# song-library Specification

## Purpose
What the songs listing says about each song, beyond what the manifest records.

## Requirements

### Requirement: A song says how long its song map is

Each song of the library listing SHALL carry `duration`, in seconds to the millisecond: the length of the audio as the song map reads it (the decoded samples), which is the length a script has to cover. It SHALL be measured the first time it is asked, remembered while the song's file does not change, and left out (the listing still works) when it cannot be measured, for example without `ffmpeg`. `audio_seconds` SHALL not change.

#### Scenario: Exact length
- **WHEN** the library lists a song whose audio decodes to 135.04 s and whose `audio_seconds` is 135.1
- **THEN** its `duration` is 135.04

#### Scenario: Remembered
- **WHEN** the library is listed twice and the file did not change
- **THEN** the audio is decoded only the first time

#### Scenario: Not measurable
- **WHEN** `ffmpeg` is missing
- **THEN** the song is listed without `duration`
