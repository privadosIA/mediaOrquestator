# image-edit-api Specification

## Purpose
TBD - created by archiving change add-generation-api. Update Purpose after archive.

## Requirements

### Requirement: The server lists the models a picture can be edited with

`GET /api/generation/options` SHALL also list the image models a picture can be edited with, in a stable order, each described well enough for a client to draw its form. The first available model SHALL be marked as the default.

#### Scenario: listing the models

- **WHEN** the options are asked for
- **THEN** every image model has an identifier, a name, a short description, its kind (`img2img`, which starts from the picture with a strength, or `instruction`, an editing model that changes the picture by its prompt), `available`, whether it accepts a strength, whether it accepts a negative prompt and the most variants it makes at once, and all identifiers are different

#### Scenario: an editing model that cannot run

- **WHEN** the files of an editing model are missing
- **THEN** it is listed with `available: false` and the reason

### Requirement: A picture is edited in one call

`POST /api/image-edits` SHALL take a picture, a model, a prompt, an optional negative prompt, an optional strength, a number of variants and an optional `client` (the name of the dashboard that asks: one to 32 characters among lowercase letters, digits and hyphens; `unknown` when it is left out), create the edit as a project of its own, launch its run on the existing queue and answer `202` with its identifier and the identifier of the run. A request the model does not accept, or a `client` that does not fit that form, SHALL be refused with `422` and the reason, and nothing SHALL be created.

#### Scenario: an img2img edit

- **WHEN** a picture is edited with an img2img model, a prompt, a strength of 0.6 and 3 variants
- **THEN** the server answers `202` and, when the run is done, there are three results

#### Scenario: an edit by instruction

- **WHEN** a picture is edited with an editing model and the prompt "put a red cap on the man"
- **THEN** the results are the same picture with the change, at the shape of the picture

#### Scenario: a setting the model does not take

- **WHEN** a strength or a negative prompt is sent to a model that does not accept it
- **THEN** the request is refused with `422` saying which setting, and nothing is created

#### Scenario: outside the limits

- **WHEN** the prompt is blank, the strength is not above 0 and at most 1, or the variants are not a whole number from 1 to the model's maximum
- **THEN** the request is refused with `422` and the reason, and nothing is created

#### Scenario: a client

- **WHEN** an edit is requested with `client` `video-studio`, in the JSON or in the `request` part of a multipart body
- **THEN** the edit, its project and its run carry `video-studio`; without `client` they carry `unknown`

#### Scenario: a client that does not fit

- **WHEN** `client` does not fit the form
- **THEN** the request is refused with `422` and nothing is created

### Requirement: The results of an edit can be read and removed

`GET /api/image-edits/{id}` SHALL give the state, the progress, its `client` (`unknown` for an edit made before clients existed), and when done the results, each with the seed that makes it again and the address of its picture. `DELETE /api/image-edits/{id}` SHALL cancel the edit when it is queued or running and remove it with its files and run history. Edits SHALL NOT appear in `GET /api/projects`.

#### Scenario: reading the results

- **WHEN** an edit is done
- **THEN** it lists one result per variant, each with its seed and the address of its picture

#### Scenario: reading the client

- **WHEN** an edit made by `video-studio` is read
- **THEN** it reports the client `video-studio`

#### Scenario: removing

- **WHEN** an edit is deleted
- **THEN** its files and run history are gone and it is no longer readable
