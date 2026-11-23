# motion-scene-review Specification

## Purpose
TBD - created by archiving change add-scene-review-loop. Update Purpose after archive.

## Requirements

### Requirement: A draft is graded against the rubric by the vision model

When a draft request carries `review` (true, or `{min, attempts}`) or the setting `music_motion.review_drafts` is on, the server SHALL, after the previews
are drawn, send the four frames composed as one picture (two by two) with the rubric's questions to the vision model routed to the `check` task, in one call,
asking first for one sentence per frame saying what a stranger would see, naming every object, and then for a score from 0 to 3 and one note per criterion
in the form `t · where · problem → exact change`, and SHALL record `review: { score 0–10 (the sum of the criteria scaled), verdict pass|weak|fail,
frames: [four sentences], criteria: [{id, score, note}], notes[] (≤ 5, concrete fixes), model }`; when a frame's sentence says the object could not be told
(a shape, bars, a blob, abstract, unclear) the metaphor's score SHALL be capped at 1 unless the video's world is declared abstract; an unreadable answer
SHALL be retried once and then recorded as `review.error` with the draft still `ready`. The rubric SHALL be one text shared with the scene writer and
printed in the docs, and its criterion on the lyric SHALL ask for big type for the words that matter (with the floors the page checks enforce) and the
keyword in the accent, and SHALL NOT ask for labels or captions: the only text of a scene is the verse, the keyword and the concept.

#### Scenario: A weak scene
- **WHEN** the previews show a clipped word and two accents
- **THEN** the review's notes name both and the verdict is `weak` or `fail`

#### Scenario: Described first
- **WHEN** a scene is graded
- **THEN** the review carries four sentences, one per frame, before the scores

#### Scenario: An object nobody can name
- **WHEN** a frame's sentence is "some grey bars over a dark ground"
- **THEN** the metaphor score is at most 1 and the note says what the picture should have been

#### Scenario: Per criterion
- **WHEN** the model answers metaphor 1, lyric 3, accent 3
- **THEN** `criteria` carries the three and `score` is 8 (7 of 9, scaled and rounded)

#### Scenario: A caption is a fault
- **WHEN** the previews show `SEC 4.37 // chorus` in a corner
- **THEN** the review's notes name it as text that does not belong to the scene

#### Scenario: Same rubric
- **WHEN** the rubric's criteria are listed from the writer's prompt and from the reviewer's prompt
- **THEN** they are the same list

### Requirement: A draft under the minimum is rewritten with the notes, a bounded number of times

With `review.min`, a draft whose score is under it SHALL be rewritten through the retry path with the notes as the problem, up to `attempts` times (default
2, max 3), keeping every attempt's score and notes in `review.attempts`; the best-scoring attempt SHALL be the draft left `ready`; nothing SHALL be
approved automatically.

#### Scenario: Two attempts
- **WHEN** `review: { min: 7, attempts: 2 }` and the first draft scores 5 and the rewrite scores 8
- **THEN** the draft is `ready` with the second version, `review.attempts` lists 5 then 8, and it is not approved

#### Scenario: Never good enough
- **WHEN** all attempts score under the minimum
- **THEN** the best one is `ready` with its review and the person decides

### Requirement: A whole piece is reviewed scene by scene and as a whole

`POST /api/music-motion/pieces/{id}/review` SHALL answer `202` and, in the background, draw three frames of each script piece's page (start, first word,
end), grade each with the rubric, grade a contact sheet of one frame per scene for consistency of type, palette and HUD, for repeated metaphors, for the
generated looks the craft names, and with two more questions: with the words covered, do the frames still tell the story, and does each frame cause the
next; and record `review: { state, scenes: [{script_piece, score, notes}], overall: {score, pictures_alone: yes|no, causes_next: yes|no, notes} }` on the
piece; the overall notes SHALL also name the scenes older than the last batch; `409` while a render or a batch runs.

#### Scenario: Repetition caught
- **WHEN** three scenes use the same stamp metaphor
- **THEN** the overall notes name the repetition and the scenes concerned

#### Scenario: The pictures alone
- **WHEN** a piece is reviewed
- **THEN** its review says whether the frames tell the story without the words and whether each frame causes the next
