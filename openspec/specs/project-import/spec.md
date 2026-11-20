# project-import Specification

## Purpose
Defines the HTTP flow that turns an external scene dataset (JSON, or HTML embedding `const data`) into a new project using the existing scene-import mapping, with a report-first preview and explicit confirmation before anything is created.

## Requirements

### Requirement: Import previews the mapping without creating anything

The server SHALL accept an uploaded dataset in the formats the `import` subcommand accepts, run the same mapping logic (not a reimplementation), and return the mapping report — per-entry mapped fields, discarded items (camera/style lines, lora_info), rejected entries with reasons, and totals — together with the generated `scenes.yaml` content. This preview step SHALL NOT create a project or write any file.

#### Scenario: preview returns report and YAML

- **WHEN** a valid dataset is uploaded for preview
- **THEN** the response contains the mapping report and the generated `scenes.yaml` content, and no project directory was created

#### Scenario: invalid dataset is rejected with the import error

- **WHEN** an uploaded file is neither a valid JSON tuple array nor HTML embedding a resolvable `const data` array
- **THEN** the response is a validation error carrying the import logic's own error message

### Requirement: Confirmation creates a project from the previewed result

The server SHALL create a new project from a confirmed import, writing the generated `scenes.yaml` into a fresh project directory named by the client. The created file SHALL be identical in content to what the preview returned and SHALL pass the pipeline's validation. An existing project identifier SHALL be rejected, never overwritten.

#### Scenario: confirm creates the project

- **WHEN** a previewed import is confirmed with a fresh project name
- **THEN** the project exists, its `scenes.yaml` equals the previewed content, and it passes validation

#### Scenario: existing name is never overwritten

- **WHEN** a confirmed import names an existing project
- **THEN** the request is rejected and the existing project is untouched

### Requirement: The preview accepts a mode

`POST /api/import/preview` SHALL accept an optional `mode` of `video` (the default) or `music`. With `music` it SHALL run the music import and answer the same fields (`yaml`, `report`, `entries`, `imported`, `rejected`) plus `instrumental`; any other mode SHALL be rejected. Confirming SHALL create the project as for any import, validating it in place, so a music project is rejected with the usual message when its models are not installed.

#### Scenario: Music preview
- **WHEN** a client posts a song dataset with `mode: "music"`
- **THEN** the answer has a music `yaml` and the per-entry report

#### Scenario: Unknown mode
- **WHEN** the mode is `audio`
- **THEN** the request is rejected as invalid

#### Scenario: Default is video
- **WHEN** no mode is sent
- **THEN** the preview behaves as before

### Requirement: The preview accepts the animated graphics mode

`POST /api/import/preview` SHALL accept `mode: "motion"` besides `video` and `music`, run the same mapping logic as the command line and answer with the report and the generated `scenes.yaml` without creating anything. Any other mode SHALL be refused with a validation error naming the accepted ones.

#### Scenario: Preview of graphics
- **WHEN** a dataset is previewed with `mode: motion`
- **THEN** the answer has a report per entry and a `scenes.yaml` of `motion_graphic` scenes, and no project exists yet

#### Scenario: Unknown mode
- **WHEN** the mode is `audio`
- **THEN** the answer is a 400 naming `video`, `music` and `motion`

### Requirement: The import can make an images project
`POST /api/import/preview` and `POST /api/import/confirm` SHALL accept an optional `output_mode` of `video` (the default) or `images`. With `images` the project's `output.mode` SHALL be `images` in the YAML the preview returns and in the project the confirm creates (overriding the YAML's own mode). Any other value SHALL be `400`; `images` with `mode: "music"` SHALL be `400`.

#### Scenario: Images dataset
- **WHEN** a client previews a scenes dataset with `output_mode: "images"`
- **THEN** the returned YAML has `output.mode: images`

#### Scenario: Default
- **WHEN** `output_mode` is not sent
- **THEN** the project is a video project as before

### Requirement: A part line is reported once
In motion mode an entry with `Part:` SHALL appear in its report's `mapped` list with one `motion_part ← <part>` line, not two.

#### Scenario: Preview of an entry with a part
- **WHEN** an entry has `Part: ticker`
- **THEN** its `mapped` list holds exactly one `motion_part` line
