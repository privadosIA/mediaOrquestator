# music-library Specification

## Purpose
TBD - created by archiving change add-song-library. Update Purpose after archive.

## Requirements

### Requirement: The library lists the songs of every music project

`GET /api/library` SHALL list the songs of the manifests of all music projects whose audio file is still on disk, most recent first, each identified by `project/track/seed`, together with the stored ratings and playlists. Projects that are not music projects or have no manifest SHALL contribute nothing.

#### Scenario: Songs across projects
- **WHEN** two music projects have exported songs and a video project exists
- **THEN** the list holds the songs of the two music projects only, without those whose file was deleted

### Requirement: Stars and playlists are stored beside the projects

`PUT /api/library/meta` SHALL replace the ratings (1 to 5, one entry per rated song) and the playlists (up to 200, each with a lowercase id, a name of up to 80 characters and up to 2000 song keys) in the database, as one change that is applied completely or not at all, and SHALL reject with 422 anything out of those limits. The stars and playlists kept in `projects/.library.json` SHALL be imported when the server starts (see `data-store`); that file SHALL NOT be taken for a project.

#### Scenario: Rating out of range
- **WHEN** a rating of 6 is sent
- **THEN** the answer is 422 and nothing is stored

#### Scenario: After the import
- **WHEN** the server starts with stars and playlists kept in the file
- **THEN** `GET /api/library` answers the same ratings and playlists as before

### Requirement: The Library screen finds, rates and plays songs

The dashboard SHALL offer a Library screen with a search over title, project, track, style, key and lyrics (ignoring case and accents, all words required), filters by project, minimum stars, favorites and rated, ordering by date, stars or title, 1 to 5 stars per song (the same stars again removing them), playlists that can be created, renamed, deleted, filled and reordered, and a "play all" that plays what the list shows song after song.

#### Scenario: Play a playlist
- **WHEN** the user opens a playlist and presses Play all
- **THEN** its first song plays and, when it ends, the next one starts, in the order of the playlist

### Requirement: Each song says who made it

Each song of `GET /api/library` SHALL carry `client` (the client of its project: its `scenes.yaml`, else `embedded`). The request SHALL accept `managed=include` (any other value is a `400`) and `client=a,b` (only songs of those clients); without them the list is what it was.

#### Scenario: Filtered by client
- **WHEN** `GET /api/library?client=music-studio` and songs of two clients exist
- **THEN** only the songs of `music-studio` are listed

### Requirement: The songs are kept in the database

The server SHALL keep each exported song as a row of the database, identified by its key (`project|track|seed`), with what the listing says about it and the duration of its audio. The rows of a project SHALL be written when a run of the project ends, from what the run exported: a new song is added, a song the run wrote again is updated, a song that is no longer exported is removed. The audio, the cover and the manifest of the run SHALL stay files. `GET /api/library` SHALL answer the same body as before from the database, without reading any folder and without measuring any audio. The songs that exist SHALL be imported when the server starts (see `data-store`). `POST /api/library/rescan` SHALL read what every project exports again and answer how many songs it added, updated and removed.

#### Scenario: After a run

- **WHEN** a music run that exports two new songs ends
- **THEN** `GET /api/library` lists them with their duration, with no request in between

#### Scenario: After the import

- **WHEN** the server starts with songs exported before
- **THEN** `GET /api/library` answers the same songs, with the same fields, as before

#### Scenario: The listing does not touch the disk

- **WHEN** `GET /api/library` is asked while the projects folder cannot be read
- **THEN** it answers the songs of the database

#### Scenario: An audio removed by hand

- **WHEN** the audio of a song is deleted from the folder and a rescan is asked
- **THEN** the song is gone from the library and the answer counts it as removed

### Requirement: The songs can be listed in pages, read and rated one by one

`GET /api/library/songs` SHALL be a list that can be paged (see `api-listing`), each song as `GET /api/library` shows it: sort by `created_at` (newest first by default), `title`, `rating` or `duration`; search `q` in the title, the caption and the lyrics; filter by `project`, `client`, `min_rating` (1 to 5), `favorite` (`true`) and `playlist` (the songs of one playlist, in its order unless `sort` is given). `GET /api/library/songs/{key}` SHALL answer one song or `404`. `PUT /api/library/songs/{key}/rating` with `{stars}` SHALL set its stars (1 to 5), or remove them with `0`; anything else SHALL be `422` and an unknown song `404`.

#### Scenario: The best of a project

- **WHEN** the list is asked with `project=demo&min_rating=4&sort=-rating`
- **THEN** only the songs of `demo` with 4 or 5 stars come back, the 5s first

#### Scenario: One rating

- **WHEN** `{stars: 5}` is put on a song
- **THEN** the song has 5 stars and no other rating changed

### Requirement: The playlists are managed one by one

`GET /api/library/playlists` SHALL list the playlists (`id`, `name`, how many songs) and `POST` SHALL create one from `{id?, name, songs?}` (`201`; the id is made from the name when it is not sent). `GET /api/library/playlists/{id}` SHALL answer it with its song keys in order, `PUT` SHALL replace its name and its songs, and `DELETE` SHALL delete it (`204`). The limits SHALL be the ones of `PUT /api/library/meta` (200 playlists, a name of up to 80 characters, 2000 songs); out of them SHALL be `422`, an id that exists `409` and an unknown one `404`. A song key that is not a song of the library SHALL be `422` naming it.

#### Scenario: A new playlist

- **WHEN** `{name: "Night drive", songs: [k1, k2]}` is posted
- **THEN** the answer is `201` with the id `night-drive` and the two songs in that order

#### Scenario: A song that does not exist

- **WHEN** a playlist is saved with a key that is not a song
- **THEN** the answer is `422` naming the key and the playlist is unchanged

### Requirement: Deleting a song or a project cleans the library

Deleting a song (`DELETE /api/projects/{id}/songs/{track}/{variant}`) or a project SHALL remove the rows of its songs, their stars and their place in every playlist, in the same change. The import SHALL leave out, and report as skipped, the stars and the playlist entries of songs that no longer exist.

#### Scenario: A project deleted

- **WHEN** a project whose songs are rated and in a playlist is deleted
- **THEN** the library has none of its songs, the playlist no longer lists them and the other songs of the playlist keep their order
