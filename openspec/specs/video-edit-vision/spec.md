# video-edit-vision Specification

## Purpose
The two networks (person mask and depth) that run in a service of their own, and the tools that use them: removing the background and making a depth map.

## Requirements

### Requirement: The background of a video is removed

The tool `remove-background` SHALL keep the people of the video and replace the rest, giving an MP4 on a plain colour (`background`: a colour, the default `#00ff00`) or, with `alpha: true`, a WebM with an alpha channel. The length, the frame rate and the audio SHALL stay the same. A video with no person in it SHALL finish saying that it found none, not as a failure, with the result unchanged.

#### Scenario: on a plain colour

- **WHEN** a video of a person is processed with `background` `#00ff00`
- **THEN** the person is on green in every frame, with the original's length and sound

#### Scenario: with alpha

- **WHEN** `alpha` is true
- **THEN** the result is a WebM whose frames are transparent outside the people

#### Scenario: nobody in the video

- **WHEN** no person is found in any frame
- **THEN** the edit ends in `done` with a note that none was found

### Requirement: A depth map of a video is made

The tool `depth` SHALL give a video, of the same size, length and frame rate, whose frames are the depth of each frame of the source as greys, near white and far black. The scale SHALL be the same for the whole clip, so that an object that does not move does not change its grey from frame to frame.

#### Scenario: a still object

- **WHEN** a camera is fixed on a still object for the whole clip
- **THEN** its grey is the same, within a small tolerance, in the first and the last frame

#### Scenario: the result is a video

- **WHEN** a clip of 33 frames is processed
- **THEN** the result has 33 frames of the same size as the source

### Requirement: The vision models run in a service the user starts

The models of `remove-background` and `depth` (and the control video of `motion-control`) SHALL run in a service of their own, started and stopped from the Environment screen like the other services, with the model files looked for in the models folder. While it is stopped, or when its files are missing, the tools that need it SHALL be listed with `available: false` and the reason, and the rest of the server SHALL work as always.

#### Scenario: the service is stopped

- **WHEN** the vision service is not running
- **THEN** `remove-background`, `depth` and `motion-control` are listed unavailable with the reason, and `scale`, `interpolate`, `reframe` and `extend` are not affected

#### Scenario: a missing model

- **WHEN** a model file is not in the models folder
- **THEN** the tool that needs it says which file is missing

#### Scenario: no Python

- **WHEN** the vision service is installed
- **THEN** it needs no Python, only the .NET runtime and the ONNX Runtime libraries
