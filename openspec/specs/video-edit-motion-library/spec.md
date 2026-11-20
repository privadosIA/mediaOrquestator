# video-edit-motion-library Specification

## Purpose
The library of motions: short videos of people moving, prepared once and applied by identifier to any picture.

## Requirements

### Requirement: The server keeps a library of motions

The server SHALL keep a library of motions: short videos of people moving (a dance, a walk, a gesture) that `motion-control` can apply to any reference picture. `GET /api/motions` SHALL list them with an identifier, a title, optional tags, the duration, the number of people, the state (`processing`, `ready`, `failed`) and the address of a small preview video; `GET /api/motions/{id}` SHALL give one and `GET /api/motions/{id}/preview` its preview. The library SHALL start empty: the server ships no videos.

#### Scenario: an empty library

- **WHEN** the library is listed on a server where none was added
- **THEN** the answer is an empty list, not an error

#### Scenario: listing

- **WHEN** motions exist
- **THEN** each one has the fields above, the most recent first, and only `ready` ones can be used

### Requirement: A motion is added from a video the user gives

`POST /api/motions` SHALL take a video (`multipart/form-data`, part `request` with `title` and optional `tags`, part `video`; or the address of a video the server can already read, as the sources of a project do), answer `202` with the motion in the state `processing`, and prepare it in the background: its control frames (the depth of the people with the background at black, at 16 frames per second, in the shape of the video: 832x480, 480x832 or 640x640) and its preview. The video SHALL last at least 2.1 seconds (one window of the control) and at most 30, and have at least one person in it; otherwise the request is refused with `422` and the reason, and nothing is kept. `DELETE /api/motions/{id}` SHALL remove it.

#### Scenario: adding a motion

- **WHEN** a video of 8 seconds with a person dancing is added with a title
- **THEN** the answer is `202` with the state `processing`, and later `ready` with the duration, the number of people and a preview

#### Scenario: a video that is too long or has nobody

- **WHEN** the video lasts more than 30 seconds or less than 2.1, or no person is found in it
- **THEN** the request is refused with `422` and the reason, and no motion exists

#### Scenario: the vision service is stopped

- **WHEN** a motion is added while the vision service is not running
- **THEN** the request is refused with `503` and the reason, and nothing is kept

#### Scenario: removing

- **WHEN** a motion is deleted
- **THEN** its files are removed and it is no longer listed

### Requirement: A motion is used by its identifier

`POST /api/video-edits` with the tool `motion-control` SHALL take either a `video` or the identifier of a motion (`motion`) together with the reference picture. With a motion, the control SHALL be the one prepared when it was added, taken from `start` (seconds, default 0) for 33 frames and fitted into the shape of the picture with black bars (the colour of the control's background), so no vision work is repeated and a long motion can be used in windows. A request with both a video and a motion, with neither, with a motion that does not exist or is not `ready`, or with a `start` that leaves fewer than 33 frames SHALL be refused with `422` and the reason, and nothing SHALL be created.

#### Scenario: a motion and a picture

- **WHEN** a ready motion and a picture are sent to `motion-control`
- **THEN** the server answers `202` and the result is the picture's subject doing the motion, without the vision service doing any work for the control

#### Scenario: a window of a long motion

- **WHEN** a motion of 10 seconds is used with `start` 4
- **THEN** the control is the 33 frames from second 4 and the result says which window was used

#### Scenario: a motion that is not usable

- **WHEN** the motion is unknown, still `processing` or `failed`
- **THEN** the request is refused with `422` and nothing is created

### Requirement: A motion remembers the address it came from

A motion added from a YouTube address SHALL keep that address (in its canonical form) as `source`, and the answers that show a motion (the one that adds it, the list and the read) SHALL carry it. A motion added from an uploaded file has no `source`. The video itself SHALL NOT be kept: the address is the only trace of where the movement comes from.

#### Scenario: From an address
- **WHEN** a motion is added with a YouTube address
- **THEN** the answer, the list and the read show that address as `source`, and the motion's folder holds no video

#### Scenario: From a file
- **WHEN** a motion is added by uploading a file
- **THEN** it has no `source`

### Requirement: The record of a motion is kept in the database

The server SHALL keep the record of each motion (its identifier, title, tags, address, duration, frames, people, size, state and error) as a row of the database; its control frames and its preview SHALL stay files. The motions that exist SHALL be imported when the server starts (see `data-store`). `GET /api/motions` SHALL be a list that can be paged (see `api-listing`): sort by `created_at` (the most recent first by default, as today) or `title`, search `q` in the title and the tags, and filter by `state`. `PATCH /api/motions/{id}` SHALL change the `title` and the `tags` sent, with the limits they have when a motion is added, and answer the motion; it SHALL be `400` for another field and `404` for an unknown motion. A motion that was being processed when the server stopped SHALL be marked as failed or interrupted, as today, when the server starts.

#### Scenario: After the import

- **WHEN** the server starts with motions kept as files
- **THEN** `GET /api/motions` answers the same motions and each preview is still served

#### Scenario: A new title

- **WHEN** `{title: "Slow walk"}` is patched
- **THEN** the motion has that title and its tags are unchanged

#### Scenario: A run uses a motion

- **WHEN** a video edit names a ready motion by its identifier
- **THEN** the run finds its record in the database and its control frames in its folder
