# motion-scene-parts Specification

## Purpose
TBD - created by archiving change add-scene-parts. Update Purpose after archive.

## Requirements

### Requirement: A scene is written from the piece, not from a pasted sentence

`POST /api/music-motion/part-drafts` SHALL accept `scene: { piece, script_piece, brief? }`; the server SHALL build the writer's input from the saved
piece: the script piece's lines with their words and times in the piece's seconds and their keyword, the hit, the section tag and index, the mode, the
resolved design (and theme when there is one), the duration, the parts of the neighbouring pieces, the scene rubric and, when the piece names a reference
with shots, the archetype of the reference shot at the nearest relative position of the video (its length, whether it cut on the beat and, when the shot
was read, what was on screen, the size of the text, how it entered and the camera), written as one line and given as a cue for the kind of shot, never as
something to copy; the brief SHALL be at most 500 characters. An unknown piece or script piece SHALL be `404`. The draft SHALL be written with
`sceneSystem`, SHALL be checked and previewed like any draft, and SHALL be refused when it does not move or, owning the verse, does not show a word at its
time.

#### Scenario: The lyric is the input
- **WHEN** a scene is asked for script piece `p7`, whose line is "Greg used to run billing" with the keyword "billing"
- **THEN** the writer's user message carries those words with their times and the keyword, and the request body carried none of them

#### Scenario: The reference's cue
- **WHEN** the piece names a reference whose shot at the same relative position was read as a huge single word entering letter by letter on a static camera
- **THEN** the writer's user message carries one line saying so as a cue, and no frame of the reference

#### Scenario: No reference
- **WHEN** the piece names no reference
- **THEN** the writer's input is as before

#### Scenario: A word shown late
- **WHEN** the written scene owns the verse and the frame at the second word's start equals the frame 100 ms before it
- **THEN** the draft is refused with "the word … did not appear at its time" and the writer is asked again with that problem

### Requirement: A scene may own the verse and the hit

A part's manifest MAY declare `"verse": "own"` and `"hit": "own"`; for a script piece whose main part owns the verse the page SHALL NOT draw `#verse`,
and likewise `#hitbg`/`#hit` for the hit; the part SHALL find the lines and the hit in `window.__scene`. Extras SHALL NOT own either. A part that owns
neither SHALL be drawn as today.

#### Scenario: No page verse
- **WHEN** the main part of a script piece declares `verse: own`
- **THEN** its page has no `#verse` element and `window.__scene.lines` carries the lines

#### Scenario: An extra cannot own it
- **WHEN** a part with `verse: own` is used as an extra
- **THEN** `PUT /script` is refused naming the piece and the part

### Requirement: Scene parts belong to their piece and stay out of the catalog's default list

A scene part SHALL be a part of the user's named `scene-<short>-<n>` with `"scene": { "piece", "script_piece" }` in its manifest; `GET /catalog` SHALL
omit scene parts unless `include=scene` (all) or `piece=<id>` (that piece's); `GET /parts/{name}` SHALL read them; deleting the piece SHALL delete its
scenes; a script piece of another piece SHALL NOT use it (`422` naming both pieces); within its piece it MAY be used by any script piece.

#### Scenario: Hidden by default
- **WHEN** `GET /catalog` is called after a scene was approved
- **THEN** the scene is not listed; with `?piece=<id>` it is

#### Scenario: Wrong piece
- **WHEN** `PUT /pieces/{other}/script` names `scene-p1-3` as a part
- **THEN** the answer is `422` and the reason names `scene-p1-3`, its piece and the other piece

### Requirement: Scenes for a whole script are written in the background and approved with their previews

`POST /api/music-motion/pieces/{id}/scenes` (optional `script_pieces`, `only: "failed"`, `brief_mode`, `model`, `provider`) SHALL answer `202` and write one draft per
script piece in order, reporting `scenes: { state, done, total, drafts[] }` on the piece; `409` while a proposal, a render or another batch runs;
`DELETE` cancels after the current draft. `only: "failed"` SHALL select the script pieces whose last batch left them with an earlier scene. Each draft SHALL be approved as any draft; `POST /pieces/{id}/scenes/approve` SHALL approve every `ready`
draft of the batch and set each script piece's `part` to its scene, validating and saving the script as `PUT /script` does, and SHALL refuse when
the script changed since the batch started, naming the script piece. A script piece whose drafts all failed SHALL be reported in the batch with `kept_previous: true` and the last problems, and the piece review SHALL name the pieces whose scene is older than the last batch.

#### Scenario: Batch with progress
- **WHEN** a script of 12 pieces is sent to `scenes`
- **THEN** the piece reports `done` rising to 12 and twelve drafts with previews, none approved

#### Scenario: Approve all
- **WHEN** `scenes/approve` is called with ten drafts `ready` and two `failed`
- **THEN** ten script pieces point to their scenes, the two keep their parts, the answer lists both groups, and the two read `kept_previous: true`

#### Scenario: Only the failed ones
- **WHEN** `scenes` is called with `only: "failed"` after that batch
- **THEN** only those two script pieces are written again

### Requirement: A scene carries no captions, draws big, and keeps the design's system

A scene draft SHALL be refused by the page checks, with the reason, when, at the middle of the scene: its visible text contains a word that is in none of its lines, its keyword, the design's concept, the song's title or the brief (a section name, a timestamp, a counter, a route number: captions belong to the `hud-chrome` part); any visible text is under 2 vh; owning the verse, a verse word is under 4 vh or its largest text is under 12 vh; or the visible elements that are not text cover less than 25 % of the frame. Text sizes SHALL be measured on the rendered box of the text (so a word enlarged with a transform counts at the size it is seen), and the message for a small headline SHALL name the keyword and the CSS that reaches the floor. A page check whose failing texts all belong to the house layers (`#verse`, `#hit`) SHALL be recorded as the server's problem, logged, and SHALL NOT fail the draft nor spend an attempt. Before the browser, a scene's CSS and SVG SHALL be refused when a `font-family` does not reference `var(--display)` or `var(--body)`, or when a colour is a literal rather than `var(--bg)`, `var(--fg)`, `var(--accent)`, a mix or a relative colour of them, `transparent` or `currentColor`, except a neutral shadow (black or white with an alpha) inside `box-shadow`, `text-shadow` or `drop-shadow()`; the theme's assets are exempt. The writer's instruction SHALL state these rules with their numbers, and the writer's input SHALL carry the one-sentence descriptions of the neighbouring scenes and of every approved scene of the piece under "already used, do not repeat".

#### Scenario: A caption
- **WHEN** a scene shows `VERSE // 21` in a corner
- **THEN** it is refused naming `VERSE // 21` and saying scenes carry no captions

#### Scenario: A word grown with a transform
- **WHEN** the keyword is set at `font-size: 6vh` with `transform: scale(2.5)` and is seen at 15 vh
- **THEN** it meets the 12 vh floor

#### Scenario: A small headline, told how
- **WHEN** the largest rendered text is 6 vh and the keyword is `night`
- **THEN** the message names `night` and says to set `font-size: 14vh` or a scale on its element

#### Scenario: The house hit overflows
- **WHEN** the only text cut by the edge is the server's own word hit
- **THEN** the draft is not refused, the problem is logged as the server's and no attempt is spent

#### Scenario: Tiny verse
- **WHEN** a scene that owns the verse draws its words at 2.5 vh
- **THEN** it is refused for verse words under 4 vh

#### Scenario: Mostly empty
- **WHEN** at the middle of the scene the picture is a few dots covering 8 % of the frame
- **THEN** it is refused as mostly empty

#### Scenario: A literal colour
- **WHEN** the fragment's CSS has `color: #ff3366`
- **THEN** it is refused naming the literal and the three variables allowed

#### Scenario: A neutral shadow
- **WHEN** the CSS has `box-shadow: 0 2vh 4vh rgba(0,0,0,0.5)`
- **THEN** it passes the colour rule

#### Scenario: A mix of the accent
- **WHEN** the CSS has `background: color-mix(in oklch, var(--accent) 30%, transparent)`
- **THEN** it passes the colour rule

#### Scenario: Metaphors already used
- **WHEN** scenes p4 and p8 are written one after the other
- **THEN** p8's writer input lists p4's description under the metaphors already used

### Requirement: The scene writer may use the timeline contract

The instruction given to the writer of a scene (and of a part draft) SHALL say that a scene may animate with one paused GSAP timeline registered in `window.__timelines`, with elements that exist for a while marked `class="clip" data-start data-duration`, in the piece's seconds, keeping `__partSeek(t, beat)` for what reacts to the beat, core GSAP only; and SHALL state the three rules the checks enforce (no tween of display/visibility on a clip, no CSS transform on what GSAP transforms, no `repeat: -1`). It SHALL also state the motion rules: entrances ease out and exits ease in and shorter, a hold of at least a beat before a hit, nothing linear but a steady drift, one punch-in on the strongest beat of the scene, and every change caused by a word, a beat or a section. A scene written with a timeline SHALL be checked, previewed and reviewed like any other; the rubric's criteria SHALL NOT change.

#### Scenario: A scene with a timeline
- **WHEN** the writer answers a scene that registers a timeline and marks two clips
- **THEN** it is drawn by the same page, passes or fails the same checks, and its preview shows the tweens at the four frames

#### Scenario: A plugin
- **WHEN** a scene references `SplitText`
- **THEN** the check fails naming the missing plugin and the writer is asked again with core GSAP only

### Requirement: A scene draft is repaired before it is refused

Before the checks of a scene draft run on an answer of the assistant, the server SHALL repair what the checks would refuse and a program can fix without changing what the scene shows: a literal colour in a style, an SVG attribute or a custom property SHALL become `var(--bg)`, `var(--fg)` or `var(--accent)`, whichever is closest to it in the design's colours (an `rgba()` or `hsla()` keeps its opacity as a percentage in a `color-mix()` of that variable with `transparent`), a font of its own SHALL become `var(--display)`, and `Math.random()` SHALL become a seeded generator defined at the start of the scene's script. A colour the program cannot read (a name other than black and white) and a shadow of black or white that the check already allows SHALL be left as they are. The repair SHALL only run on an answer that has something the checks refuse, and the repaired answer SHALL go through the same checks as any answer. A draft that still fails SHALL go back to the assistant with its remaining problems as before, and an answer SHALL count as an attempt only when it fails the checks after being repaired.

#### Scenario: A colour literal is repaired
- **WHEN** an answer has `background: linear-gradient(0deg, rgba(0,0,0,0.8) 0%, transparent 100%)` in a dark design
- **THEN** the draft is checked with `color-mix(in srgb, var(--bg) 80%, transparent)` in its place and no attempt is spent on the colour

#### Scenario: Nothing to repair
- **WHEN** an answer has no colour literal, no font of its own and no `Math.random()`
- **THEN** its page is the assistant's, byte for byte

#### Scenario: The model is still asked when the repair is not enough
- **WHEN** a repaired answer still has a text that is never drawn
- **THEN** the assistant gets that problem and the attempt counts

### Requirement: A draft says what was repaired

A draft SHALL carry `repairs`, a list of `{kind, count}` (`colour`, `font`, `clock`) with the number of places each one changed, empty when nothing was repaired. The preview, the review and the part that is approved SHALL be those of the repaired page. The scene batch's report of each draft SHALL carry the same list.

#### Scenario: Repairs are listed
- **WHEN** a draft was repaired in 3 colours and 1 font
- **THEN** its `repairs` is `[{kind: "colour", count: 3}, {kind: "font", count: 1}]`

#### Scenario: Nothing repaired
- **WHEN** an answer passes the checks as it is
- **THEN** its `repairs` is empty and its page is the assistant's, byte for byte
