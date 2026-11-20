# motion-part-drafts Specification

## Purpose
Lets the user ask for a new part of the catalog from a picture and a sentence, as a draft that is checked like any part, previewed, edited and approved by the user.

## Requirements

### Requirement: A draft is asked for from a sentence and, optionally, a picture

The server SHALL accept a request for a draft with a `request` (what the part has to do, 1 to 500 characters) and, optionally, a picture: a key frame of a reference of the library (`reference_id` and `frame`, one of the names in its analysis) or an uploaded picture (png, jpg or webp, up to 20 MB). It SHALL answer `202` with the draft in the state `writing`, with `with_picture` saying whether there is a picture, do the work in the background and set the state to `ready` or `failed` (with the reason). It SHALL refuse with `422` and create nothing a request with no sentence, an unknown reference or frame or a file that is not a usable picture, with `409` while a run is queued or active and the model is local, and with `503` and the reason when there is no Chrome or no assistant, or when a picture is given and there is no vision model (a picture that was given SHALL NOT be dropped to go on without it). A draft with a picture SHALL keep its own copy of it on the server only, and SHALL NOT publish it or use it to generate pictures or video.

#### Scenario: A frame of a reference
- **WHEN** a draft is asked for with a frame of a reference and a sentence
- **THEN** the answer is `202` with the state `writing`, and a later read says `ready` or `failed`

#### Scenario: Only a sentence
- **WHEN** a draft is asked for with a sentence and no picture, and there is no vision model
- **THEN** the answer is `202` with `with_picture` false, and the draft is written from the sentence alone

#### Scenario: Nothing to do it with
- **WHEN** there is no assistant or no browser, or a picture is given and there is no vision model
- **THEN** the answer is `503` with the reason and no draft exists

#### Scenario: Not usable
- **WHEN** the sentence is empty, the frame is not one of the reference's or the file is not a picture
- **THEN** the answer is `422` with the reason and nothing is created

### Requirement: A picture goes through the age check first

When there is a picture, before anything is written, it SHALL go through the age check of the descriptor that describes references. A person who is, or may be, a minor, or whose age cannot be confirmed, SHALL stop the draft (state `failed`, saying why), and the picture SHALL be deleted from the draft; a refusal of the model SHALL be reported and never replaced by an invented text.

#### Scenario: A minor
- **WHEN** the picture shows a person who is or may be a minor
- **THEN** the draft is `failed` with the reason, no part is written and the picture is not kept

### Requirement: The assistant writes a part and the draft is checked like any part

The assistant SHALL write the part (a `manifest.json` and a `part.html`) from the description of the picture (when there is one), the sentence and the rules of a part. The draft SHALL pass the same checks as the parts that come with the program: the manifest rules, the fragment rules (it loads nothing from outside and does not take over the page's seek), the checks of a motion page, movement over a beat (not the same frame all over it, judged with no verse on the page), equal frames in two renders and no text cut by the edge of the frame. The name SHALL be a new one: a name of the catalog is a problem. An answer that fails SHALL be asked again with the problems, up to the usual number of attempts; when none passes the draft SHALL be `failed` with the last manifest and fragment and the last problems, and nothing SHALL be invented to fill it.

#### Scenario: A valid part
- **WHEN** the assistant answers with a part that passes the checks
- **THEN** the draft is `ready` with its manifest, its fragment and the attempts it took

#### Scenario: A refused answer is asked again
- **WHEN** the first answer loads something from outside
- **THEN** the assistant is asked again with that reason and a later valid answer makes the draft `ready`

#### Scenario: Nothing valid
- **WHEN** no attempt passes the checks
- **THEN** the draft is `failed` with the last problems and the last files, which the user can edit

### Requirement: A ready draft has a preview

A draft that passes the checks SHALL have a preview: frames drawn over one beat with the colours of a design. The preview SHALL be fetchable by its file names, listed in the draft.

#### Scenario: A preview
- **WHEN** a draft is `ready`
- **THEN** its read lists the preview frames and each can be fetched as a picture

### Requirement: Drafts are managed

The server SHALL list the drafts (newest first), read one with its manifest, fragment, state, attempts, problems and preview, replace its manifest and fragment, ask for another attempt, and delete it with its files (which stops a running job). A replacement SHALL be checked like a draft: it is refused with `422` and the problems when it does not pass, and saved, with a new preview, when it does. An unknown draft SHALL be `404`. A draft that was `writing` when the server stopped SHALL be `failed` (interrupted) after the restart.

#### Scenario: Edit
- **WHEN** the fragment of a draft is replaced by one that passes the checks
- **THEN** it is saved, the preview is drawn again and the draft is `ready`

#### Scenario: A bad edit
- **WHEN** the fragment is replaced by one that loads a picture from outside
- **THEN** the answer is `422` with the problems and the draft is as it was

#### Scenario: Another attempt
- **WHEN** another attempt is asked for on a `failed` draft
- **THEN** the assistant writes the part again from the same sentence and picture (if it had one)

### Requirement: Only approving adds a part to the catalog

A draft SHALL NOT be in the catalog. Approving a `ready` draft SHALL check it again, copy its manifest and fragment to the user's parts folder, where the catalog lists it, and delete the draft, answering with the part. It SHALL be refused with `409` when a part of that name is already in the user's folder and with `422` when the draft does not pass. Nothing SHALL approve a draft by itself.

#### Scenario: Approve
- **WHEN** a ready draft is approved
- **THEN** the catalog lists the part, the draft is gone and the answer holds the part

#### Scenario: A name that is taken
- **WHEN** the user's folder already has a part with the draft's name
- **THEN** the answer is `409` and nothing changes

#### Scenario: Not approved
- **WHEN** a draft is ready and nobody approves it
- **THEN** the catalog does not list it

### Requirement: A part remembers the video it came from

A draft made from a key frame of a reference, or proposed by a learning from a reference, SHALL keep the reference's id, its title and the address of its video, so that they outlive the reference. When such a draft is approved, the part SHALL keep them as its `origin` (with the key frame, whether a learning proposed it and which one, and when it was approved) in a file beside its manifest, and the catalog SHALL show that `origin` on every read. A part that comes with the program, and a part whose draft came from a sentence or an uploaded picture alone, SHALL have no `origin`.

#### Scenario: A frame of a reference
- **WHEN** a draft made from a frame of a reference is approved after the reference was deleted
- **THEN** the part's `origin` still gives the reference's title and the address of its video

#### Scenario: Learned
- **WHEN** a part that a learning proposed is approved
- **THEN** its `origin` says it was learned and names the learning and the reference

#### Scenario: Nothing to remember
- **WHEN** a draft written from a sentence alone is approved
- **THEN** the part has no `origin` and the catalog lists none for it

### Requirement: The model that writes a draft can be chosen

`POST /api/music-motion/part-drafts` SHALL accept `model`, in the JSON body or as a `model` field of the multipart form: the name of an installed model of the assistant's provider that writes the part instead of the one the motion-graphics task is routed to. A model that is not installed SHALL be `422` naming it and the installed ones, before any draft is created. The draft SHALL keep the model (`model`, absent when none was chosen) and a retry of a failed draft SHALL write with it. The model that reads a picture SHALL NOT be affected.

#### Scenario: A model that is not installed
- **WHEN** a draft is asked for with a model that is not installed
- **THEN** the answer is `422` naming the model and the installed ones, no draft is created and no writer is built

#### Scenario: The chosen model writes the part
- **WHEN** a draft is asked for with an installed model
- **THEN** the writer is built for that model and the draft says `model`

#### Scenario: A retry keeps it
- **WHEN** a failed draft that chose a model is tried again
- **THEN** the writer is built for the same model

#### Scenario: No model
- **WHEN** a draft is asked for without `model`
- **THEN** the routing's own model writes it and the draft has no `model`

### Requirement: A draft can be asked for a scene of a piece

Besides a sentence and a picture, a draft SHALL be askable for a scene with `scene: { piece, script_piece, brief? }`; such a draft SHALL be written
with the scene instruction and the piece's own context built by the server, SHALL carry `scene` in its record, and SHALL otherwise live, be previewed,
retried, edited, approved and deleted like any draft.

#### Scenario: A scene draft is a draft
- **WHEN** a scene draft is `ready` and `approve` is called
- **THEN** a part of the user's with the `scene` field exists and the draft is marked approved

### Requirement: A draft may carry a review

A draft's record MAY carry `review` as the `motion-scene-review` capability describes, and a draft request MAY ask for it with `review`; `retry` SHALL
accept the review's notes as the problem when the loop rewrites. Drafts without `review` SHALL be unchanged.

#### Scenario: Read back
- **WHEN** `GET /part-drafts/{id}` is called for a reviewed draft
- **THEN** the answer has `review.score`, `review.verdict`, `review.notes` and `review.attempts`

### Requirement: Drafts are kept in the database

The server SHALL keep each draft as a row of the database with its state, its request, its problems, its review, its manifest and its fragment (and the ones it replaces, for a change of a part); its picture and its preview frames SHALL stay in files. The drafts that exist as files SHALL be imported when the server starts (see `data-store`). `GET /api/music-motion/part-drafts` SHALL be a list that can be paged (see `api-listing`): sort by `created_at` (the order of today by default), search `q` in the request and the name, and filter by `state` and by `piece` (the drafts of the scenes of one piece). A draft that was being written when the server stopped SHALL be marked as interrupted when the server starts.

#### Scenario: After the import

- **WHEN** the server starts with 315 drafts kept as files
- **THEN** the list answers the same drafts as before and each preview frame is still served

#### Scenario: The failed drafts of a piece

- **WHEN** the list is asked with `piece=<id>&state=failed&limit=20`
- **THEN** at most 20 failed drafts of that piece come back, with the total in `X-Total-Count`

#### Scenario: A restart during a draft

- **WHEN** the server is stopped while a draft is being written and started again
- **THEN** the draft is `interrupted` before any request is answered
