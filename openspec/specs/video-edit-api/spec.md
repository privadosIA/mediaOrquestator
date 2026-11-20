# video-edit-api Specification

## Purpose
The one-call API that edits a video that already exists: the list of tools with what each needs, and the creation, follow-up, listing and removal of an edit as a hidden project on the run queue.

## Requirements

### Requirement: The server lists the tools a video can be edited with

`GET /api/video-edits/options` SHALL list the tools in a stable order (`scale`, `interpolate`, `reframe`, `remove-background`, `depth`, `extend`, `motion-control`), each with an identifier, a name, a short description, `available`, the reason when it is not, the settings it accepts (name, kind, limits, default) and whether it needs a reference picture. A tool that needs files or a service that are not there SHALL be listed with `available: false` and the reason; the list SHALL NOT fail.

#### Scenario: listing the tools

- **WHEN** the options are asked for
- **THEN** every tool has the fields above and all identifiers are different

#### Scenario: a tool that cannot run

- **WHEN** the vision service is stopped or the model of a tool is missing
- **THEN** the tools that need it are listed with `available: false` and the reason, and the others are not affected

### Requirement: A video is edited in one call

`POST /api/video-edits` SHALL take a video, a tool, the settings of that tool, an optional reference picture and an optional `client` (one to 32 characters among lowercase letters, digits and hyphens; `unknown` when left out), create the edit as a project of its own, launch its run on the existing queue and answer `202` with its identifier and the identifier of the run. The request is `multipart/form-data`: a part `request` with the JSON and a part `video` (mp4, mov, webm or mkv), and a part `image` (png, jpg, jpeg or webp) when the tool takes one. A request the tool does not accept SHALL be refused with `422` and the reason, `503` when the tool cannot run now, and in both cases nothing SHALL be created.

#### Scenario: a valid edit

- **WHEN** a video is sent with the tool `scale` and a factor of 2
- **THEN** the server answers `202` with the identifier of the edit and of its run

#### Scenario: a setting outside its limits

- **WHEN** a setting is not one the tool accepts (an unknown factor, an aspect ratio that is not listed, a frame rate that is not 24, 30 or 60)
- **THEN** the request is refused with `422` saying which setting, and nothing is created

#### Scenario: a file that is not a video

- **WHEN** the `video` part is not a video the machine can read
- **THEN** the request is refused with `422` and nothing is created

#### Scenario: a tool that cannot run now

- **WHEN** the tool is listed with `available: false`
- **THEN** the request is refused with `503` and the reason, and nothing is created

#### Scenario: a missing reference picture

- **WHEN** the tool needs a reference picture (`motion-control`) and none is sent, or one is sent to a tool that does not take it
- **THEN** the request is refused with `422` and nothing is created

#### Scenario: a client

- **WHEN** an edit is requested with `client` `video-studio`
- **THEN** the edit, its project and its run carry `video-studio`; without `client` they carry `unknown`

### Requirement: An edit is followed, listed and removed

`GET /api/video-edits` SHALL list the edits, the most recent first; `GET /api/video-edits/{id}` SHALL give one with its tool, its settings, its state (`queued`, `running`, `done`, `failed`, `cancelled`), its progress, the identifier of its run, its size on disk, the address of its result and, when it failed, the reason. `DELETE /api/video-edits/{id}` SHALL cancel it if it is running and remove its files and its project. Edits SHALL NOT appear in `GET /api/projects`, the storage per project or the statistics per project.

#### Scenario: following an edit

- **WHEN** an edit is running
- **THEN** its state is `running` and its progress goes up with the run's events, and when it ends its state is `done` and the result is served as an artifact

#### Scenario: a failed edit

- **WHEN** the tool fails (the source cannot be decoded, a model fails to load)
- **THEN** its state is `failed` with the reason, and no partial result is offered

#### Scenario: removing a running edit

- **WHEN** an edit that is running is deleted
- **THEN** its process stops, its files and its project are removed and it is no longer listed

#### Scenario: hidden from projects

- **WHEN** projects are listed after an edit was made
- **THEN** the edit is not among them

### Requirement: Every edit gives an MP4 of the same clip, never a fragment of what the machine could not finish

The result of an edit SHALL be an MP4 (H.264, `yuv420p`), except `remove-background` with alpha, which gives a WebM with alpha. The audio of the source SHALL be kept by the tools that do not change the length or the speed of the video (`scale`, `reframe`, `remove-background`) and dropped by the others, and the result SHALL say which it has.

#### Scenario: the audio of a scale

- **WHEN** a video with sound is scaled
- **THEN** the result has the same sound

#### Scenario: no audio when the picture changes speed

- **WHEN** the frame rate of a video is raised, or a video is extended
- **THEN** the result says whether it has sound, and it has the original's only when it still matches the picture

### Requirement: One heavy process at a time

The edits SHALL go through the run queue of the server, so that no two processes that use the GPU run together and a user who starts an edit while a generation is running is told so, as every other run is.

#### Scenario: an edit while a generation runs

- **WHEN** an edit that uses the GPU is launched while another run uses it
- **THEN** the answer carries the same notice as any other run, and the edit waits its turn
