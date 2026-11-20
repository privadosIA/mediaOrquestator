# api-documentation Specification

## Purpose
Defines the server's OpenAPI description of its own HTTP API: served only behind the API's normal authentication and kept in step with the registered routes.

## Requirements

### Requirement: The server describes its API in OpenAPI

The server SHALL serve an OpenAPI 3.0 document at `GET /api/openapi.json` that describes every endpoint it registers: method, path, path and query parameters, request body, the success response and the error responses with the standard error shape. The document SHALL be part of the binary (no file read at runtime) and SHALL declare the Bearer security scheme.

#### Scenario: the description is served

- **WHEN** a client requests `GET /api/openapi.json` with a valid token (or with no token configured on the server)
- **THEN** the answer is `200` with `Content-Type: application/json` and a document whose `openapi` field starts with `3.0`

#### Scenario: streaming endpoints are described

- **WHEN** the description is read for `GET /api/runs/{id}/events`
- **THEN** it declares a `text/event-stream` response and says that the endpoint is meant for `EventSource`, not for a one-shot request

### Requirement: The description is only reachable with the API's authentication

`GET /api/openapi.json` SHALL be subject to the same authentication as every other `/api/*` endpoint, with no exemption. The server SHALL NOT register any other public route that serves the description or an interactive documentation page.

#### Scenario: anonymous request with a token configured

- **WHEN** the server runs with a token and `GET /api/openapi.json` arrives without credentials
- **THEN** the answer is `401` with the standard error shape and no part of the description

#### Scenario: no public documentation route

- **WHEN** a client requests `/swagger`, `/docs` or `/openapi.json` outside `/api/` on a server with a token
- **THEN** none of them serves the description or Swagger UI

### Requirement: The description cannot drift from the registered routes

The project SHALL include a test that compares the routes the server registers with the paths and methods of the description, and fails naming every route that is registered but undocumented and every documented route that is not registered.

#### Scenario: a route is added without documenting it

- **WHEN** a handler is registered for a new method and path and the description is not updated
- **THEN** the test fails and names that route

#### Scenario: the description lists a removed route

- **WHEN** the description contains a method and path no handler is registered for
- **THEN** the test fails and names that route

### Requirement: The description covers music mode

The OpenAPI description SHALL document everything music mode adds or changes in the API: the `music` mode value and the `tracks` list in project payloads, the `music` asset kind in the model, catalog and delete endpoints, the `ace-lm` and `ace-synth` entries of the environment report, the `compose` and `synth` phase names in run events, and audio media types on artifact responses (including `206` for range requests). The route-drift test SHALL keep passing with no undocumented route.

#### Scenario: Mode enumeration
- **WHEN** the description is read for a project payload
- **THEN** the allowed `mode` values include `music`

#### Scenario: Music kind in the catalog
- **WHEN** the description is read for the catalog and model endpoints
- **THEN** the `music` kind appears in their schemas

#### Scenario: Audio artifact
- **WHEN** the description is read for the artifact endpoint
- **THEN** it lists audio media types and the `206` partial-content response

### Requirement: The description covers the music assistant task

The OpenAPI description SHALL list `music` among the tasks of `POST /api/assist`, document the optional `instrumental` request field and the `caption` and `lyrics` response fields.

#### Scenario: Task and fields are described
- **WHEN** the description is read for `POST /api/assist`
- **THEN** it lists `music`, `instrumental`, `caption` and `lyrics`

### Requirement: The description covers music adapters

The OpenAPI description SHALL list `adapter` among the asset kinds of the assets, downloads and delete endpoints, mention that its files may be named `<folder>/<file>`, and describe the `music_adapters` field of the environment report.

#### Scenario: Kind and field are described
- **WHEN** the description is read
- **THEN** it lists the `adapter` kind and the environment's `music_adapters`

### Requirement: The description covers the import mode

The OpenAPI description SHALL document the optional `mode` of `POST /api/import/preview` with its values and the extra `instrumental` count in the answer.

#### Scenario: Mode is described
- **WHEN** the description is read for the import preview
- **THEN** it lists `mode` with `video` and `music`

### Requirement: The source endpoints are documented

The OpenAPI document SHALL describe the source endpoints and the task fields of a track.

#### Scenario: Sources in the document
- **WHEN** the document is read
- **THEN** it contains `/api/projects/{id}/sources` and `/api/projects/{id}/sources/{name}` with their statuses

### Requirement: The video upload is documented

The OpenAPI description of the sources upload SHALL state that a video is converted to an mp3 source and list its extra statuses.

#### Scenario: Document
- **WHEN** the document is read
- **THEN** the upload mentions video conversion and answers 422 and 503

### Requirement: The sources document pictures

The OpenAPI description of the sources endpoints SHALL state that pictures are accepted.

#### Scenario: Document
- **WHEN** the document is read
- **THEN** the upload mentions `.png`, `.jpg`, `.jpeg` and `.webp`

### Requirement: The YouTube source endpoint is documented

The OpenAPI document SHALL describe `POST /api/projects/{id}/sources/from-url` with its body (`url`, optional `name`), the `201` answer and the `400`, `409`, `422` and `503` statuses.

#### Scenario: Document
- **WHEN** the document is read
- **THEN** it contains the path with those statuses

### Requirement: The peaks endpoint and the mixes are documented

The OpenAPI document SHALL describe `GET /api/projects/{id}/peaks` with its parameters, its answer and its statuses.

#### Scenario: Document
- **WHEN** the document is read
- **THEN** it contains the path with `file` and `buckets` and the statuses 400, 404, 422 and 503

### Requirement: The description declares the fields of every answer a client reads

The OpenAPI description SHALL declare, for every endpoint whose answer a client reads field by field, the fields of that answer: their names, types, which are always present (required) and which are only sometimes present or can be `null`. A success response SHALL NOT be declared as "any object" (an object with no properties). This covers at least the environment report (`GET /api/environment`), the statistics (`GET /api/stats`) and the resources sampled during a run (`GET /api/runs/{id}/resources`). The description SHALL be written so that a client generated from it, and a simulated server built from it, have a type for each of those answers.

#### Scenario: the environment report is typed

- **WHEN** the description is read for `GET /api/environment`
- **THEN** the success response names the fields of the report (edition, content mode, binaries, models, voices, LoRAs, music files, GPUs, the Ollama address, the keys status, the disk and the memory) and says which of them the server always sends

#### Scenario: the statistics are typed

- **WHEN** the description is read for `GET /api/stats`
- **THEN** the success response names the period, the totals by state, the durations, the days, the phases, the cache, the groups by project, GPU and mode, and the series of runs, each with its fields

#### Scenario: the resources of a run are typed

- **WHEN** the description is read for `GET /api/runs/{id}/resources`
- **THEN** the success response names the sampling interval, whether the samples were cut, the GPU and memory samples (time, used memory and, for a GPU, utilisation) and the peaks, whose memory peak can be `null`

#### Scenario: an optional field is not required

- **WHEN** the server leaves a field out of an answer in some cases (the disk when it cannot be read, a service key source when there is no key)
- **THEN** the description does not list that field as required

### Requirement: The declared fields cannot drift from the real answers

The project SHALL include a test that obtains the answers of the real handlers for the endpoints of the previous requirement and compares them with the description: it SHALL fail naming the field when an answer carries a field the description does not declare, when a field the description marks as required is missing, or when a value does not have the declared type.

#### Scenario: a field is added to an answer without describing it

- **WHEN** a handler starts sending a field its description does not declare
- **THEN** the test fails and names the endpoint and the field

#### Scenario: a described field is no longer sent

- **WHEN** a handler stops sending a field the description marks as required
- **THEN** the test fails and names the endpoint and the field

### Requirement: The settings answer is described in full

The OpenAPI description SHALL declare every block of the answer of `GET /api/settings` and `PUT /api/settings` (the Ollama address, the optional HeartMuLa and DiffRhythm2 services, the cap of calls to a remote provider, the browser of the motion graphics and the model that writes them) with their fields, and the same test that walks the other answers against the description SHALL walk this one against the real handler. The summary of the endpoint SHALL NOT limit it to the Ollama address.

#### Scenario: a client reads the settings

- **WHEN** the description is read for `GET /api/settings`
- **THEN** the success response names the `ollama`, `heartmula`, `diffrhythm2`, `remote`, `browser` and `motion_model` blocks and the fields of each

#### Scenario: a block is added to the settings without describing it

- **WHEN** the handler starts sending a block the description does not declare
- **THEN** the test fails and names the block

### Requirement: Fields that an answer always carries are marked as required

For every schema of an answer a client reads, the OpenAPI description SHALL declare every field the server can send and SHALL mark as required exactly those it always sends. A field the server sends only in some cases (`omitempty`, or behind a pointer that can be absent) SHALL be optional, and a field that is always present and can be `null` SHALL say so. A request body whose fields are all optional SHALL say that in its description instead of leaving the reader to guess.

#### Scenario: a client reads a project

- **WHEN** the description is read for the schema of a project in the list of projects
- **THEN** `id`, `name`, `mode` and `scene_count` are required, and `track_count`, `adult`, `invalid` and `error` are optional

#### Scenario: a project that cannot be read

- **WHEN** a project's file cannot be read and the list of projects is requested
- **THEN** the entry is still described by the schema: `mode` may be empty, `name` empty and `scene_count` 0, with `invalid` and `error` present

#### Scenario: a run is read

- **WHEN** the description is read for the schema of a run
- **THEN** it declares every field a run carries, and the ones the server always sends (`id`, `project`, `state`, `gpu`, `keep_tmp`, `force`, `vram_warn_mb`, `created_at`, `result_available`) are required

#### Scenario: a request whose fields are all optional

- **WHEN** the description is read for the body of a monitoring tool's settings
- **THEN** it says that every field may be left out, and what leaving each one out means

### Requirement: The description follows the structures that build the answers

The project SHALL include a test that compares each schema of the previous requirement, and the objects nested in it, with the Go structure that builds the answer: it SHALL fail naming the schema and the field when a field of the structure is not declared, when a declared field is not in the structure, when a field is required and the structure may omit it, or when a field is always sent and is not required.

#### Scenario: a field is added to a structure without describing it

- **WHEN** a field is added to the structure of an answer and not to the description
- **THEN** the test fails and names the schema and the field

#### Scenario: a field is made optional in the structure

- **WHEN** a field of the structure gets `omitempty` and the description still marks it as required
- **THEN** the test fails and names the schema and the field

### Requirement: Every error code the server uses is in the description

The `code` of the error body SHALL list every code the server can answer, and a test SHALL fail when a code the server defines is not in the list.

#### Scenario: a new error code

- **WHEN** a new error code is defined in the server and not added to the description
- **THEN** the test fails and names the code

#### Scenario: a client reads the codes

- **WHEN** the description of the error body is read
- **THEN** `describe_refused`, `consent_required`, `too_many_calls` and the other codes the server uses are among the values of `code`

### Requirement: The description uses only the keywords of OpenAPI

Every schema of the OpenAPI description SHALL use only keywords OpenAPI defines (or `x-` extensions), and a test SHALL fail naming the schema and the key otherwise, so that a description whose text was split into keys by a YAML mistake cannot be served.

#### Scenario: a description with an unquoted comma

- **WHEN** a one-line description contains a comma and is not quoted, so that part of its text becomes a key
- **THEN** the test fails and names the schema and the key

### Requirement: The description declares the ticket as a security scheme

The OpenAPI document SHALL declare a `ticket` security scheme (an API key in the `ticket` query parameter) besides the Bearer scheme, and every operation that accepts a ticket SHALL list both schemes in its `security`; no other operation SHALL list the ticket. A test SHALL compare the server's list of ticket routes with the operations that declare the scheme and fail on any difference.

#### Scenario: a ticket route
- **WHEN** the operation `GET /api/reviews/{rid}/findings/{fid}/frame` is read
- **THEN** its `security` lists `bearer` and `ticket`

#### Scenario: a route added to the ticket list without the document
- **WHEN** the server accepts a ticket on a route whose operation does not declare the scheme
- **THEN** the test fails naming the route
