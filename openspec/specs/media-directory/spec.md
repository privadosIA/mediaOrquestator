# media-directory Specification

## Purpose
Lets the user choose one directory, possibly on another disk, where whole projects (configuration, sources, cache and results) are stored, while the paths the API and the manifests use do not change.

## Requirements

### Requirement: A project lives as one folder in the media directory

When a media directory is configured (`--media-dir` or `MEDIA_DIR`), the server SHALL keep each project in `<media>/<project>/` with `config/` (`scenes.yaml`, its backups `scenes.yaml.*`, `sources/`, `.reviews/`), `out/` and `tmp/`, and SHALL make `scenes.yaml`, `sources`, `.reviews`, `out` and `tmp` of the project resolve to them, so the project looks the same to the pipeline, to manifests, to runs and to the API (`out/<name>/…`, `out/clips/…`). Without the setting the project SHALL stay whole inside the projects directory as before.

#### Scenario: A new project
- **WHEN** a project is created with a media directory configured
- **THEN** `<media>/<project>/config/scenes.yaml` exists and the project reads and saves its scenes through it

#### Scenario: Same paths for the client
- **WHEN** a run finishes and its artifact is `out/<project>/manifest.json`
- **THEN** that path is served by the API exactly as without a media directory

#### Scenario: Cache on the media disk
- **WHEN** a run writes intermediate files
- **THEN** they are written under `<media>/<project>/tmp/` and nothing grows on the disk of the server

#### Scenario: No setting
- **WHEN** no media directory is configured
- **THEN** the project is stored inside `projects/<p>/` as before

### Requirement: The media directory is checked at start

The server SHALL create the media directory if it is missing and SHALL refuse to start, naming the directory and the cause, when it cannot be created or written. If the directory is not available while the server runs (for example the disk is unmounted), a run SHALL fail with a message naming it, and SHALL NOT write the project into the local disk instead.

#### Scenario: Unwritable directory
- **WHEN** the configured directory cannot be written
- **THEN** the server does not start and the message names the directory

#### Scenario: Disk gone
- **WHEN** the media disk is unmounted and a run is launched
- **THEN** the run is refused with a message naming the directory and nothing is written locally

### Requirement: Existing projects are migrated without loss

A command SHALL move the existing files of every project to the media directory: copy, check that every file arrived with its size, and only then replace the originals by links. It SHALL be safe to run again, SHALL support `--dry-run` (listing what would move and the space needed) and SHALL refuse to run when the media disk lacks the space. It SHALL NOT delete the original of a project whose copy did not verify, SHALL NOT move a project with a queued or running run, and SHALL leave the `models` and `voices` links as they are.

#### Scenario: Migration
- **WHEN** a project has real `scenes.yaml`, `out/` and `tmp/` and the migration runs
- **THEN** its files are in `<media>/<project>/`, the project paths are links and a manifest reads the same files

#### Scenario: Repeat
- **WHEN** the migration runs again
- **THEN** nothing changes and it reports nothing to do

#### Scenario: Not enough space
- **WHEN** the files to move need more than the free space of the media disk
- **THEN** nothing is moved and the message says how much is needed and how much is free

### Requirement: Deleting, exporting and importing a project follow the media folder

Deleting a project SHALL also remove `<media>/<project>/`. Exporting a project SHALL include its configuration, sources and results as today, and importing SHALL write them into the media directory. The server SHALL never remove or write outside `<media>/<project>/` through the links.

#### Scenario: Delete
- **WHEN** a project is deleted
- **THEN** its folder in the media directory is gone and the folders of other projects are not touched
