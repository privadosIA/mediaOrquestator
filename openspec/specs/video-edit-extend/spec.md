# video-edit-extend Specification

## Purpose
Making a video longer with a new clip, made by an engine that takes a first frame, that starts where the video ends.

## Requirements

### Requirement: A video is made longer

The tool `extend` SHALL make the video longer by a new clip that starts at the last frame of the video, with a prompt that says what happens next (required), and join the two. The seconds asked for (2 to 10) are the most the clip lasts: a clip is what the engine makes by itself (about 2 seconds with MiniMax H3 or LTX), so when more is asked the video grows by the engine's clip and the result says so. The engine SHALL be the one the request names among those that take a first frame (`minimax-h3`, `ltx`) and are available, the first available one by default. The new clip SHALL have the size and the frame rate of the video, and the join SHALL NOT repeat the last frame. A video SHALL be extendable again from its result.

#### Scenario: an extension

- **WHEN** a 5 second video is extended by 2 seconds with a prompt, on an engine whose clip lasts 2 seconds or more
- **THEN** the result is about 7 seconds, the first 5 are the source unchanged and the next 2 start where the source ends

#### Scenario: more than the engine makes

- **WHEN** 4 seconds are asked and the engine's clip lasts 2.3
- **THEN** the video grows by 2.3 seconds and the result says that the clip is shorter than asked

#### Scenario: no engine can run

- **WHEN** none of the engines that take a first frame is available
- **THEN** the tool is listed with `available: false` and the reason

#### Scenario: a request outside the limits

- **WHEN** the seconds are not from 2 to 10, or the prompt is blank
- **THEN** the request is refused with `422` and nothing is created

#### Scenario: the sound

- **WHEN** the extended video is made
- **THEN** it has no sound unless the engine made it for the new clip, and the result says which

### Requirement: The extension is honest about its limit

The result of `extend` SHALL carry the number of times the video has been extended from its original, and the options SHALL say that the quality drops with each extension.

#### Scenario: an extension of an extension

- **WHEN** an already extended video is extended again
- **THEN** the second result says it has been extended twice
