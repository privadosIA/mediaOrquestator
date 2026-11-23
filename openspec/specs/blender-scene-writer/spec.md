# blender-scene-writer Specification

## Purpose
TBD - created by archiving change add-blender-scene-writer. Update Purpose after archive.

## Requirements

### Requirement: A model writes the description of a 3D scene from an idea

`POST /api/projects/{id}/blender/descriptions` with `{name, idea, seconds, revise?, provider?}` SHALL ask the language model of the routed `motion_graphics` task for a description (contract version 1 of `blender-engine`) of a scene `seconds` long, and SHALL store it as `sources/<name>.scene.json` only when it passes the same checks as one written by hand. `name` SHALL be a plain file name ending in `.scene.json`, `idea` a text of up to 2000 characters and `seconds` a number from 0.5 to 600. The model SHALL be told, from the project and from the contract and not from a fixed text: the length of the scene and the frame's aspect, the colours behind `bg`, `fg` and `accent` of the project's design, the fonts `display` and `body` only when the design has a font in the server's pack for them, the tempo and the number of beats the scene lasts or, when the project has no tempo, that keys are in seconds, and the object types, light types, easings, properties and limits the contract accepts. The idea SHALL reach the model as data to illustrate and never as an instruction that can change what is accepted: the answer is stored only if it passes the checks, whatever the idea says. The answer of the call SHALL carry the stored description as written, the same resolved (every key in seconds, every colour `#rrggbb`), the provider and model that wrote it and the number of attempts. An existing file of that name SHALL be replaced only when the new description is accepted.

#### Scenario: An idea becomes a scene
- **WHEN** the call is made with the idea "the word Hola in big letters rising from the floor, a red ball that hits on every beat, a slow dolly in" and `seconds: 5` in a project with a design and a tempo of 120 bpm
- **THEN** `sources/<name>.scene.json` exists, passes the checks of `blender-engine`, and the answer says which model wrote it and in how many attempts

#### Scenario: The writer uses only what the project has
- **WHEN** the project has no `defaults.motion.bpm`, or its design has no font of the server's pack for `display`
- **THEN** the model is told to key in seconds, or not to set a font, and a description that uses `beat` or `font: "display"` anyway is asked for again with the problem

#### Scenario: The idea tries to change the rules
- **WHEN** the idea says to ignore the contract and answer with something else
- **THEN** nothing but a description that passes the checks is stored, and the call fails with `422` if the model obeyed

#### Scenario: The name is not a plain file name
- **WHEN** `name` is `../x.scene.json`, an absolute path, or does not end in `.scene.json`
- **THEN** the call is refused with `400` and the model is not called

### Requirement: A description that does not pass is asked for again and then refused

When the model's answer is not valid JSON, breaks a rule of the contract, or is a scene with no object or in which nothing moves, the server SHALL ask again with the problems, each naming its place in the JSON, and the description it sent, up to 3 attempts in all. When no attempt passes it SHALL answer `422` with the last problems and SHALL leave the stored file as it was. A scene SHALL count as having something that moves when the camera or an object has at least two keys that differ in a channel they both give.

#### Scenario: Repaired at the second attempt
- **WHEN** the first answer has a key beyond the length of the scene and the second does not
- **THEN** the second is stored and the answer says 2 attempts

#### Scenario: Nothing moves
- **WHEN** the answer is a valid description with one key on the camera and one on each object, all alike
- **THEN** it is asked for again, naming that nothing moves

#### Scenario: Three failures
- **WHEN** no attempt passes
- **THEN** the answer is `422` with the problem of the last attempt, and a file that was stored before is as it was

### Requirement: A stored description can be revised in words

With `revise: true` the model SHALL be shown the stored description and the request in `idea`, and SHALL answer with the whole new description, which goes through the same checks, attempts and storing as a new one. A call with `revise: true` for a name with no stored file SHALL be refused with `404`.

#### Scenario: Slower camera
- **WHEN** a description is stored and the call asks with `revise: true` for a slower camera, and the model answers with a valid description
- **THEN** the stored file is the new one and only the model's answer decides what changed

#### Scenario: Nothing to revise
- **WHEN** `revise` is true and `sources/<name>.scene.json` does not exist
- **THEN** the call is refused with `404` and the model is not called

### Requirement: A description can be read and saved by hand

`GET /api/projects/{id}/blender/descriptions/{name}` SHALL return the stored file as written and resolved, and `PUT` with `{description, seconds}` SHALL store a description written by hand after the same checks as the writer's (against a scene `seconds` long, as the writer's call does; the length is not stored, and the render checks the description again against the real length of the scene), answering `400` with the problems (each naming its place in the JSON) when it does not pass and leaving the stored file as it was. `GET /api/projects/{id}/sources` SHALL list these files with `kind: scene`. A project whose scene says `blender.build: <name>` SHALL validate once the file is stored.

#### Scenario: Edited by hand
- **WHEN** a description written by the model is read, changed and saved with `PUT`
- **THEN** it is stored if it passes, and a scene of the project that names it renders as usual

#### Scenario: A faulty description saved by hand
- **WHEN** `PUT` sends a description with a key of `t: 9` for a scene of 5 seconds
- **THEN** the answer is `400` naming `objects[0].keys[2].t` and the stored file is as it was

### Requirement: A frame of a description can be previewed cheaply

`POST /api/projects/{id}/blender/descriptions/{name}/preview` with `{t, seconds, width?}` SHALL render the single frame at second `t` of the stored description (checked against a scene `seconds` long, like the writer's call) at a width of up to 960 pixels (480 by default, the project's aspect) with few samples, on the CPU so that it never takes a card from a run, and SHALL answer with the PNG. It SHALL refuse with `503`, saying how to install it, when Blender is not available, and with `400` when `t` is outside `seconds` or the description is not valid, and with `404` when there is no such description.

#### Scenario: A frame to look at
- **WHEN** the preview of a stored description is asked at `t: 1.5`
- **THEN** the answer is a PNG of the scene as it looks 1.5 seconds in, and no card of a run is used

#### Scenario: No Blender
- **WHEN** Blender is not installed
- **THEN** the writer and the revisions still work, and the preview is refused with `503` and the way to install it

### Requirement: The writer follows the routing, the consent and the runs of the model it uses

The call SHALL use the provider of the `motion_graphics` task (or the `provider` of the body), SHALL refuse with the routing errors the other model calls give (a remote provider without consent, a cap of calls reached), and SHALL answer `409` while a run is queued or active when the model loads into this machine's memory. It SHALL answer `503` when there is no model.

#### Scenario: A remote model without consent
- **WHEN** the provider of the task is remote and the user has not consented
- **THEN** the call is refused as every other model call is and nothing is sent

#### Scenario: A run is active
- **WHEN** a run is active and the model is a local one that needs the machine's memory
- **THEN** the call answers `409` naming the run
