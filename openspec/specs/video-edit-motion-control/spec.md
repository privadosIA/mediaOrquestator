# video-edit-motion-control Specification

## Purpose
Moving the movement of the people of a video (or of a motion of the library) onto the people of a reference picture.

## Requirements

### Requirement: The motion of the people of a video is moved onto another picture

The tool `motion-control` SHALL take a video of people (or the identifier of a motion of the library, see `video-edit-motion-library`) and a reference picture, and make a video of 33 frames (about 2 seconds at 16 per second) in the shape of the picture (832x480 for a landscape one, 480x832 for a portrait one, 640x640 for one that is near square, and 832x480 when its shape is not known), in which what the picture shows moves the way the people of the video do, with the background of the picture and not the video's. It SHALL take a prompt, a strength (0.2 to 1, the default 0.6) of how much the motion rules and a guidance (1 to 10, the default 6) of how closely the prompt is followed; a lower guidance burns the highlights less and follows the prompt less. It is meant for subjects with a human outline: the options SHALL say so.

#### Scenario: two people to two other people

- **WHEN** a video of two people and a picture of two other people are given
- **THEN** the result shows the people of the picture doing the movements of the video, in the picture's place

#### Scenario: the background is not copied

- **WHEN** a video of people in a park is used with a picture set indoors
- **THEN** the result is indoors, not in the park

#### Scenario: the shape of the picture

- **WHEN** the reference picture is portrait
- **THEN** the result is 480x832, and the control of the video is fitted into that shape with black bars, never cut

#### Scenario: a source longer than the clip

- **WHEN** the video lasts more than 2 seconds
- **THEN** the control is taken from its first 33 frames at 16 per second, and the result says which part was used

#### Scenario: a source with nobody

- **WHEN** no person is found in the first 33 frames
- **THEN** the request is refused with `422` saying so, and nothing is made

### Requirement: Motion control says what it needs

`motion-control` SHALL be listed unavailable, with the reason, when the vision service is not running or the model files of the control (depth, person mask) or of the video model are missing, or when the graphics card has less memory than the tool needs; it SHALL say which of them.

#### Scenario: the video model is missing

- **WHEN** the video model file is not in the models folder
- **THEN** the tool is listed with `available: false` and says that file is missing

#### Scenario: a card with little memory

- **WHEN** no graphics card has the memory the tool needs
- **THEN** the tool is listed unavailable with the memory it needs and the one there is
