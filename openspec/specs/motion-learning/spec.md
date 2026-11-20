# motion-learning Specification

## Purpose
Lets the person learn from a reference video in one operation: the server proposes the styles and parts that are different from what the catalog already has, keeps nothing hidden and adds nothing to the catalog by itself.

## Requirements

### Requirement: Learning starts from a reference that is ready

The server SHALL accept `POST /api/music-motion/references/{id}/learn` for a reference whose state is `ready`, with an optional `strictness` (`low`, `normal` or `high`, `normal` by default). It SHALL answer `202` with a learning (id, reference, state `running`, `ready` or `failed`, the reason when it failed, the strictness used) and do the work in the background. A reference that does not exist SHALL answer `404`; one that is not `ready`, `409`; a `strictness` that is not one of the three, `422`. While a learning of the same reference is running, a new call SHALL answer `200` with that learning and start nothing.

#### Scenario: A ready reference
- **WHEN** learning is asked for a `ready` reference
- **THEN** the answer is `202` with the state `running`, and a later read says `ready` or `failed`

#### Scenario: Not ready or unknown
- **WHEN** learning is asked for a reference that is still being analysed, or for one that does not exist
- **THEN** the answer is `409` or `404` and nothing is created

#### Scenario: Already running
- **WHEN** learning is asked again for a reference whose learning is still `running`
- **THEN** the answer is `200` with the same learning and no second one starts

#### Scenario: Not a strictness
- **WHEN** `strictness` is `extreme`
- **THEN** the answer is `422` with the accepted values

### Requirement: A learning proposes what is different

A finished learning SHALL list, from the analysis of the reference, at most one style and at most three part ideas, each with `novel`, and `similar_to` naming the closest thing that exists (a style or a part of the catalog) with a distance and a short reason. A proposal that is `novel` SHALL be kept (a style, or the draft of a part) and listed under `styles` or `part_drafts`. A proposal that is not SHALL NOT be created and SHALL be listed under `discarded` with the same fields and the reason it was discarded. What the first call of a reference made SHALL count as existing for the next one, so a second call does not keep the same style or the same part again (the assistant may propose other ideas, which are kept when they are different).

#### Scenario: Something new
- **WHEN** the style drawn from a reference is far enough from every style that exists
- **THEN** it is stored and listed under `styles` with `novel` true and its closest style

#### Scenario: Too similar
- **WHEN** a part idea is too close to a part of the catalog
- **THEN** no draft is made and the idea is listed under `discarded` with `similar_to` naming that part and the reason

#### Scenario: Learning twice
- **WHEN** a reference is learned from, and once finished it is learned from again with nothing else changed
- **THEN** the style the first call kept is discarded as similar, and a part idea is kept only if it is also different from the drafts the first call made

#### Scenario: Strictness moves the line
- **WHEN** the same reference is learned from with `low` and with `high`
- **THEN** `high` discards at least everything that `low` discards

### Requirement: Nothing enters the catalog by itself

A style a learning keeps SHALL be a normal style (it can be edited, deleted and used to make a piece) marked `origin: learned` and naming its reference. A part a learning proposes SHALL be a draft marked `origin: learned` and naming the learning, and SHALL stay a draft until the person approves it. What was made by hand SHALL carry no `origin`, and a client that never asks to learn SHALL see no change in any answer.

#### Scenario: A learned style is an ordinary style
- **WHEN** a learning keeps a style
- **THEN** the styles list shows it with `origin` `learned`, and it can be edited and deleted like any other

#### Scenario: A learned part waits
- **WHEN** a learning keeps a part draft
- **THEN** the catalog does not list the part until the draft is approved, and the draft says it was learned

#### Scenario: By hand
- **WHEN** a style or a draft is made as before
- **THEN** its answer carries no `origin`

### Requirement: Parts are written and checked like any draft

The parts a learning proposes SHALL be written and checked as the drafts of `motion-part-drafts` are: the same assistant, the same checks of the part, the same preview, and, when a key frame of the reference is the inspiration, the same age check. A draft that fails SHALL be listed with its reason and SHALL NOT stop the rest of the learning.

#### Scenario: A part that does not pass
- **WHEN** a part idea is written and no attempt passes the checks
- **THEN** its draft is `failed` with the problems, as any draft, and the style and the other ideas are not affected

#### Scenario: A minor in the frame
- **WHEN** the frame chosen shows a person who is or may be a minor
- **THEN** that idea ends as a failed draft with the reason of the age check and no picture is kept

### Requirement: Without an assistant the style still comes

When there is no assistant, no browser or no vision model, the learning SHALL still draw the style from the analysis, SHALL leave the parts out, and SHALL say so in `skipped` (what was left out and the name of what is missing). It SHALL NOT fail for that reason.

#### Scenario: No assistant
- **WHEN** learning is asked and there is no assistant
- **THEN** the learning is `ready` with the style (if it is novel), no part drafts, and `skipped` naming the assistant

### Requirement: Learnings can be read

The server SHALL answer `GET /api/music-motion/learnings/{id}` with the learning, its proposals and the current state of each part draft, and `GET /api/music-motion/learnings` with the learnings, newest first, optionally of one reference (`reference_id`). A learning that was running when the server stopped SHALL read `failed` with that reason.

#### Scenario: Reading
- **WHEN** a learning is read while one of its drafts is still being written
- **THEN** the draft is listed with its present state, and a later read shows `ready` or `failed`

#### Scenario: After a restart
- **WHEN** the server restarts while a learning is running
- **THEN** the learning reads `failed` as interrupted and what it had already kept stays

### Requirement: Learnings are kept in the database and can be deleted

The server SHALL keep each learning as a row of the database; the ones that exist as files SHALL be imported when the server starts (see `data-store`). `GET /api/music-motion/learnings` SHALL be a list that can be paged (see `api-listing`): sort by `created_at` (the order of today by default) and filter by `reference` and `state`. `DELETE /api/music-motion/learnings/{id}` SHALL delete a learning and answer `204`; it SHALL be `404` for an unknown one and `409` while it is running, and it SHALL never delete the style or the parts the learning proposed. A learning that was running when the server stopped SHALL be marked as interrupted when the server starts.

#### Scenario: Deleting a finished learning

- **WHEN** a finished learning is deleted
- **THEN** it is gone from the list and the style it proposed is still there

#### Scenario: Deleting one that is running

- **WHEN** a running learning is deleted
- **THEN** the answer is `409` and it keeps running
