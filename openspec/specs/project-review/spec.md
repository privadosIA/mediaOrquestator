# project-review Specification

## Purpose
TBD - created by archiving change add-project-review. Update Purpose after archive.

## Requirements

### Requirement: A project can be analyzed after a run

The Review page of a project SHALL offer an **Analyze** button that checks the animated clips of the project as a job with visible progress and stores the result as a review. The vision model SHALL be chosen on the page among the models Ollama reports as able to read images (the last one used is remembered); without one the analysis SHALL measure colour drift and sharpness only and say so. A project with no animated clips made yet SHALL say there is nothing to analyze. The vision model SHALL be unloaded when the analysis ends.

#### Scenario: Analyze
- **WHEN** the user presses Analyze on a project whose clips exist
- **THEN** a review appears with a progress indicator and, when done, its findings

#### Scenario: Nothing to analyze
- **WHEN** the project has no animated clips yet
- **THEN** the page says so and does not start a job

### Requirement: Findings say what was noticed and how it will be fixed

A review SHALL list its findings, each with its scene, kind, the second from which it appears, severity, what was noticed, a frame as evidence, its source (automatic or the user's) and its **proposed fix** with its status. The proposed fix SHALL be: cut a silent scene to its clean part, or make the scene again with another seed when it is bad from the start or its clip cannot be cut. The user SHALL be able to change the proposed fix or skip a finding.

#### Scenario: A deformed tail
- **WHEN** a silent scene's clip is clean for 1.2 s and deformed after
- **THEN** its finding shows the frame, the issue and the proposed fix "cut to 1.2 s"

#### Scenario: Skip
- **WHEN** the user skips a finding
- **THEN** it keeps its place in the list, marked skipped, and is not run with the others

### Requirement: Fixes run as tasks

Each finding SHALL be runnable as a task, alone, in a selection or all pending at once. Running a task SHALL apply its fix to the project's `scenes.yaml` (keeping its comments and a backup) and make the scene again through the run queue limited to that scene; the finding's status SHALL follow the run (`running`, `done`, `failed`) and link to it. The page SHALL offer **Analyze again** to check what changed. A task SHALL NOT start while another run of the project is active.

#### Scenario: Run one
- **WHEN** the user runs the task of one finding
- **THEN** scenes.yaml is edited for that scene, a run limited to it is queued and the finding shows running, then done

#### Scenario: Run all
- **WHEN** the user runs all pending tasks
- **THEN** they run one after another through the queue and each shows its own status

#### Scenario: Busy project
- **WHEN** another run of the project is active
- **THEN** the task is refused saying so

### Requirement: The user can add what they saw

The page SHALL let the user add a finding of their own: the scene, the seconds it refers to, what they saw, a severity and how to fix it. The fixes SHALL include cutting, a new seed and rewriting the motion prompt; for the last, a local text model MAY propose the new motion prompt from what the user wrote, as a suggestion the user can edit before running. The finding SHALL be marked as the user's and run like any other.

#### Scenario: A manual observation
- **WHEN** the user adds "faces melt from 1.3 s" for a scene and chooses "calmer motion"
- **THEN** a finding of theirs appears with a proposed motion prompt they can edit and run

### Requirement: Reviews can be deleted

Reviews SHALL be kept in the history and SHALL be deletable one by one, with a confirmation, and in bulk (all of a project's, or those older than a date), refusing and counting the ones whose analysis or tasks are running. Deleting a review SHALL delete its evidence frames. The list SHALL show how many reviews there are.

#### Scenario: Delete one
- **WHEN** the user deletes a finished review and confirms
- **THEN** it and its evidence are gone

#### Scenario: Bulk with one running
- **WHEN** the user deletes all of a project's reviews and one is still analyzing
- **THEN** the others are deleted and the answer says one was skipped

### Requirement: Analyses finish or fail visibly and are waited for at shutdown

A review whose analysis cannot save its result SHALL end in the state `failed` with the error, never stay `analyzing`. On shutdown the server SHALL wait for running analyses (up to the shutdown timeout) before closing the database.

#### Scenario: the database is unavailable
- **WHEN** an analysis finishes and saving its findings fails
- **THEN** the review is `failed` with the error and the client stops seeing `analyzing`

#### Scenario: shutdown during an analysis
- **WHEN** the server is stopped while an analysis runs
- **THEN** the analysis is cancelled or finished before the database closes and no write happens afterwards
