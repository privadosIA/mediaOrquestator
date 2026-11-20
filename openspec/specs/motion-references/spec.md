# motion-references Specification

## Purpose
Keeps the videos the user gives as examples as references the server analyses by itself, so what makes them good can be seen, searched and used.

## Requirements

### Requirement: A reference is added from an address or a file

The server SHALL accept a reference from a YouTube address (as the sources of a project do, with no account and no cookies) or from an uploaded video, with a `title`, optional `notes` and optional `tags`. It SHALL answer `202` with the reference in the state `analysing`, make the analysis in the background and set the state to `ready` or `failed` (with the reason). It SHALL refuse with `422` an address it cannot use, a file that is not a video, and a video longer than 20 minutes, and nothing SHALL be created. The video SHALL NOT be kept once analysed: only its key frames and the analysis. A reference SHALL be kept on the server only and SHALL NOT be used to generate pictures or video.

#### Scenario: An address
- **WHEN** a reference is added from an address
- **THEN** the answer is `202` with the state `analysing`, and a later read says `ready` with its analysis

#### Scenario: Not usable
- **WHEN** a reference is added with an address that is not supported or a file that is not a video
- **THEN** the answer is `422` with the reason and no reference exists

#### Scenario: The video is not kept
- **WHEN** the analysis is done
- **THEN** the folder of the reference holds its frames and its analysis and not the video

### Requirement: The analysis says what the video is like

The analysis SHALL give: the duration, the frame rate and the size; between 12 and 24 key frames spread over the video; the five dominant colours (as hex colours with their share) of the whole video and of each third of it; the shots of the video, whose boundaries are the hard cuts found with a scene-change filter at a fixed threshold **and** the content changes found by comparing small grey pictures of the video taken twice a second (a change is a difference clearly above the video's own noise and a local peak, put at the picture where the picture jumps most inside that half second; a change within a quarter of a second of a hard cut is the same boundary), each shot with its kind (`cut` or `change`, by the boundary that starts it), its start and end, how much it moves from 0 to 1, its dominant colour and, when the audio has a clear tempo, whether its boundary falls within 60 ms of a beat; shots under 0.4 s merged into the one before and at most 600 shots (the change threshold raised first, then the change pass dropped, then the scene threshold raised in steps); the number of cuts (one less than the shots) and the mean length of a shot; a rhythm sheet made from the shots (boundaries per minute in each third and, apart, content changes per minute and the share of boundaries that are hard cuts; the time of the first boundary, the 10-second window with most boundaries, the longest shot of the last two thirds, the median and the longest shot, the median in each third, the share of boundaries on the beat when known together with the beat phase used, the share of static shots) and the sheet written as one short paragraph of text that names the kinds counted and the phase when it is not the tracked one; and, when the video has sound, the tempo and the key of the audio as estimates. The beat phase SHALL be chosen among the tracked grid and the grid shifted by a quarter, a half and three quarters of a beat as the one that puts most boundaries on the beat, only when at least 8 boundaries fall in the tracked span (else the tracked grid). It SHALL NOT use the network beyond fetching the video, and the same video SHALL give the same analysis.

#### Scenario: What is measured
- **WHEN** a reference with sound is analysed
- **THEN** its analysis has the duration, the frames, the colours, the shots with their kind, motion and beat boundaries, the rhythm sheet with its text, the cuts, the mean shot length and an estimate of the tempo

#### Scenario: A typographic video
- **WHEN** a video swaps one word for another on the same plain background every second without a hard cut
- **THEN** each swap starts a shot of kind `change`, at the time of the swap within a tenth of a second, and the sheet's boundaries per minute count them

#### Scenario: A slow pan
- **WHEN** a shot pans slowly over a picture for ten seconds
- **THEN** no change boundary is found inside it

#### Scenario: Cut on the off-beat of the tracker
- **WHEN** the boundaries of a video fall about half a beat from the tracked beats
- **THEN** the phase of half a beat is used, most boundaries read as on the beat, and the sheet says the phase

#### Scenario: No sound
- **WHEN** the video has no audio
- **THEN** the tempo and the key are absent, no shot says whether its boundary is on a beat, the sheet omits that share and the phase, and the rest is there

#### Scenario: A flash
- **WHEN** two boundaries are 0.2 s apart
- **THEN** they make one shot, not two

#### Scenario: Same video, same analysis
- **WHEN** a video is analysed twice
- **THEN** the two analyses are equal

### Requirement: A look is described when there is a model

When a model that reads pictures is available, the analysis SHALL include a short description of the look of the video (colours, typography, the kind of screens, how the text enters) written from its key frames by the descriptor that describes references, with the same safeguards: a frame with a person that is a minor, or whose age cannot be confirmed, SHALL stop the description and be deleted, and the model's refusals SHALL be reported and never replaced by an invented text. Without such a model the analysis SHALL be complete without the description, and say so.

#### Scenario: Described
- **WHEN** a vision model is available
- **THEN** the analysis has a description of the look

#### Scenario: No model
- **WHEN** no vision model is available
- **THEN** the analysis has everything but the description and says why

#### Scenario: A minor
- **WHEN** a key frame shows a person that is or may be a minor
- **THEN** the description is not made, that frame is deleted and the analysis says it

### Requirement: The library is managed

The server SHALL list the references (newest first), read one with its analysis and its frames, change its title, notes and tags, and delete it with its files. A reference that is being analysed SHALL be deletable, which stops its analysis. The routes SHALL be behind the same access as the rest of the API, an unknown reference SHALL be `404`, and the descriptions SHALL be in the OpenAPI description.

#### Scenario: Edit and delete
- **WHEN** the notes of a reference are changed and then it is deleted
- **THEN** the first answer shows the new notes, and after the second the reference and its files are gone

#### Scenario: Listed
- **WHEN** references are listed
- **THEN** they come newest first with their state, title and tags

### Requirement: The shots are read when there is a model

When a model that reads pictures is routed to describing, the analysis SHALL also read up to 40 shots spread over the video, each with one closed question answered as a small JSON: what is on screen (`typography`, `figure`, `chart`, `ui`, `photo`, `abstract`, `empty`), the text (`none`, `word`, `line`, `block`) and its size (`small`, `medium`, `huge`), how it entered (`cut`, `fade`, `slide`, `scale`, `letter`, `word`, `unknown`, judged from the frame at the shot's start and the one at its middle) and the camera (`static`, `zoom`, `pan`, `shake`). The reading SHALL run in the background after the reference is `ready`, with `analysis.breakdown_state` saying `pending`, `reading`, `done`, `skipped` (with why, when there is no model) or `failed`; it SHALL have the safeguards of the look: a frame with a person who is or may be a minor stops the reading and is deleted, a refusal is recorded and never replaced, and an answer that is not the JSON asked for is retried once and then recorded as `unknown`. With the readings the rhythm sheet SHALL add the share of text-led shots and the counts of entrances and subjects, and its text SHALL say them. The frames of the shots SHALL be kept with the reference and go with it.

#### Scenario: Read
- **WHEN** a vision model is routed and a reference becomes ready
- **THEN** its shots are read in the background, `breakdown_state` ends `done`, and the sheet says how many shots are text-led and how the text enters

#### Scenario: No model
- **WHEN** no vision model is routed
- **THEN** the shots and the sheet are there without readings and `breakdown_state` is `skipped` with the reason

#### Scenario: A minor
- **WHEN** a shot's frame shows a person who is or may be a minor
- **THEN** the reading stops, that frame is deleted, and the analysis says it

#### Scenario: Not JSON
- **WHEN** the model answers prose for a shot twice
- **THEN** that shot's reading is `unknown` and the others are kept

### Requirement: The breakdown can be run again

`POST /api/music-motion/references/{id}/breakdown` `{ "read"?: true }` SHALL compute the shots, the sheet and, with `read` (default `true`), the readings of a `ready` reference again, in the background, answering `202` with the reference; `409` while a breakdown of it is running; `404` for an unknown reference; `422` for one that is not `ready`. A reference analysed before shots existed SHALL get them this way, and nothing SHALL be recomputed on its own. The key frames, the colours, the tempo and the look SHALL NOT change.

#### Scenario: An old reference
- **WHEN** the route is called for a reference whose analysis has no shots
- **THEN** it answers `202` and the reference later has its shots and its sheet, with its key frames and look unchanged

#### Scenario: Already running
- **WHEN** the route is called while a breakdown of the same reference runs
- **THEN** the answer is `409`

### Requirement: References are kept in the database

The server SHALL keep each reference as a row of the database with its state, its title, its address and its analysis; its frames and shots SHALL stay in files. The references that exist as files SHALL be imported when the server starts (see `data-store`). `GET /api/music-motion/references` SHALL be a list that can be paged (see `api-listing`): sort by `created_at` (the order of today by default) or `title`, search `q` in the title and the address, and filter by `state`. A reference that was being analysed when the server stopped SHALL be marked as interrupted when the server starts.

#### Scenario: After the import

- **WHEN** the server starts with references kept as files
- **THEN** the list and each reference answer as before and their frames are still served

#### Scenario: Only the ready ones

- **WHEN** the list is asked with `state=ready`
- **THEN** only the references that are ready come back
