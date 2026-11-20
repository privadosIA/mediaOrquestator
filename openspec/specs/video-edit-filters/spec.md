# video-edit-filters Specification

## Purpose
The tools that need only ffmpeg: scaling (with an optional super-resolution network), raising the frame rate (with an optional interpolation network) and reframing to another aspect ratio.

## Requirements

### Requirement: A video is scaled

The tool `scale` SHALL make the frame of the video 2 or 4 times as large in each side, with the engine the request names: `fast` (a resampling filter, the default) or `quality` (a super-resolution network). The frame rate, the length and the audio SHALL stay the same. Sides SHALL be even numbers and the result SHALL NOT be larger than 8192 pixels in either side; a request that would pass that SHALL be refused with `422`.

#### Scenario: a fast scale

- **WHEN** a 832x480 video is scaled by 2 with `fast`
- **THEN** the result is 1664x960 with the same length, frame rate and sound

#### Scenario: a quality scale

- **WHEN** a video is scaled with `quality`
- **THEN** the result is the size asked for and its frames come from the network, and the run tells which engine made them

#### Scenario: a scale that is too big

- **WHEN** the result would pass 8192 pixels in a side
- **THEN** the request is refused with `422` and nothing is created

### Requirement: The frame rate of a video is raised

The tool `interpolate` SHALL make the video have 24, 30 or 60 frames per second by making the frames in between, with the same length. A target not above the video's own rate SHALL be refused with `422`. When the interpolation network cannot run, the tool SHALL still work with the fallback filter and the run SHALL tell it.

#### Scenario: from 16 to 30

- **WHEN** a video of 16 frames per second is interpolated to 30
- **THEN** the result has 30 frames per second and the same length

#### Scenario: a target that is not above

- **WHEN** a video of 30 frames per second is asked for 24
- **THEN** the request is refused with `422` and nothing is created

#### Scenario: no network

- **WHEN** the interpolation network is not installed
- **THEN** the edit still finishes with the fallback filter and says it used it

### Requirement: A video is reframed

The tool `reframe` SHALL give the video the aspect ratio asked for (`16:9`, `9:16`, `1:1`, `4:5`) by `crop`, which cuts what does not fit, or by `pad`, which adds bars of a colour, the default `crop`. With `crop` the window SHALL follow the person when there is one in the video, otherwise it SHALL be centred. The length, the frame rate and the audio SHALL stay the same. By `crop` the result SHALL be the largest window of the ratio that fits in the picture (the height of the source, or the width when that makes it smaller); by `pad` it SHALL be the smallest canvas of the ratio that holds the whole picture.

#### Scenario: a landscape video to portrait

- **WHEN** a 16:9 video is reframed to 9:16 by `crop` and a person is on the left of the frame
- **THEN** the result is 9:16, with the same length and sound, and the person is inside the window in every frame

#### Scenario: padding

- **WHEN** a 16:9 video is reframed to 1:1 by `pad` with black
- **THEN** nothing of the picture is cut and the bars are black

#### Scenario: an aspect ratio that is not listed

- **WHEN** the aspect ratio is not one of the listed
- **THEN** the request is refused with `422`
