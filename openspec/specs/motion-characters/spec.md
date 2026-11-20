# motion-characters Specification

## Purpose
Gives the music motion pieces an illustrated style made of characters and backgrounds that are kept as files, can be added to over time and are used by a script like any other part.

## Requirements

### Requirement: Characters and backgrounds are kept as files in a library

The server SHALL keep a library of characters and backgrounds. A character SHALL have a manifest (name, description, the size of its drawing, its poses and expressions by name, its anchor point) and one SVG per pose or one SVG with a group per pose. A background SHALL have a manifest and one SVG or image. The library SHALL start with a few characters and backgrounds that come with the program, which stay inside the program, and the user's SHALL be kept in the database: the manifest and the SVG drawings as text, an image that is not SVG as a file. The folders of the user's library that exist SHALL be imported when the server starts (see `data-store`), leaving out, with one warning in the log and one entry in the import record, an item whose manifest is invalid or whose file is missing. The user SHALL add, change and delete characters and backgrounds through the API, without changing the server; a folder put in the library by hand SHALL enter only when an import is asked for.

#### Scenario: Listing
- **WHEN** the catalog is requested
- **THEN** the characters list their poses and expressions and the backgrounds list their names

#### Scenario: A character added
- **WHEN** the user posts a valid character
- **THEN** the next catalog includes it and a script can use it

#### Scenario: Broken item
- **WHEN** a character is posted whose manifest names a pose with no drawing
- **THEN** the answer is `422` naming the pose and the library is unchanged

#### Scenario: A folder put by hand
- **WHEN** a valid character folder is put in the library folder and the import of the library is asked for
- **THEN** the next catalog includes it

### Requirement: A script places characters on a stage

A piece of a script MAY have a `stage`: a background, and characters each with a position, a pose, an expression, a size and an action from a fixed list (`enter`, `exit`, `bounce`, `look`, `wave`, `slide`). The actions SHALL be timed from the beat clock, so that a character moves on the beat. A character, pose, expression, background or action that is not in the library SHALL be refused naming it. A character SHALL not be drawn partly outside the frame at the middle or the last frame of its piece unless its action is `enter` or `exit`.

#### Scenario: A character bounces on the beat
- **WHEN** a piece has a character with the action `bounce`
- **THEN** the character moves once per beat of the song

#### Scenario: Unknown pose
- **WHEN** a script uses a pose the character has not
- **THEN** it is refused and the answer names the character and the pose

### Requirement: The style is optional and does not need a model for the drawings

The illustrated style SHALL be a style a piece can choose besides the typographic one. It SHALL work with the characters of the library only: no image model SHALL be loaded to draw them. The frame rate for this style SHALL default to 24.

#### Scenario: No image model
- **WHEN** a piece in the illustrated style is rendered
- **THEN** no diffusion model is loaded

#### Scenario: Default rate
- **WHEN** a piece is created in the illustrated style with no frame rate
- **THEN** it is 24 fps

### Requirement: The user's characters and backgrounds are managed through the API

`GET /api/music-motion/library/characters` and `GET /api/music-motion/library/backgrounds` SHALL list the items (of the program and of the user, each with its `source`) and can be paged (see `api-listing`). `POST` on each SHALL create one of the user's from `{manifest, files}` (`files`: the name of each drawing and its SVG text) and answer `201`; `PUT .../{name}` SHALL replace one of the user's and `DELETE .../{name}` SHALL delete it (`204`). A drawing SHALL pass the checks the library applies today (no script, no `foreignObject`, no event handler, nothing loaded from outside). It SHALL be `409` to create a name that exists, `403` to change or delete one that comes with the program, `404` for an unknown name and `422` with the problems for an item that is not valid, changing nothing.

#### Scenario: A drawing with a script

- **WHEN** a character whose SVG has a `<script>` is posted
- **THEN** the answer is `422` with that problem and nothing is saved

#### Scenario: One of the program

- **WHEN** a character that comes with the program is deleted
- **THEN** the answer is `403` and it stays

#### Scenario: Deleting one of the user's

- **WHEN** a character of the user's is deleted
- **THEN** it is gone from the catalog and from `GET /api/music-motion/library/characters/{name}`
