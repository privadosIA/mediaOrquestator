# motion-presets Specification

## Purpose
Turns a reference into a style a new piece can start from: a design, a rhythm and the parts that suit it.

## Requirements

### Requirement: A style is drawn from a reference

From a reference that is `ready` the server SHALL make a style: a design whose tones and accent come from the dominant colours of the video (a dark tone, a light one and an accent that contrasts with the background of the modes the style uses, checked as a design is), two fonts from the fixed list, a `verse_style` and a `hit_style`, a rhythm (how long a piece lasts, from the mean length of a shot, within the limits of a piece), and a list of parts of the catalog that suit it. The parts SHALL be chosen by the assistant from the description of the look and the descriptions of the parts, validated against the catalog; without an assistant, or when it fails, the list SHALL be empty and the style SHALL still be made. A style SHALL always be valid: a design that does not pass the validation SHALL be adjusted until it does, and the style SHALL say it was adjusted.

#### Scenario: A style from a reference
- **WHEN** a style is made from a reference with a description and no assistant is available
- **THEN** the style has a valid design and a rhythm and an empty list of parts

#### Scenario: With the assistant
- **WHEN** the assistant answers with parts of the catalog
- **THEN** the style's list holds them, and a part that is not in the catalog is left out

#### Scenario: Readable
- **WHEN** the colours of the video give an accent with too little contrast
- **THEN** the style's accent is adjusted until it is readable and the style says so

### Requirement: Styles are managed

The server SHALL list styles, read one, replace one after validating it (the design as a whole design, the parts in the catalog, the rhythm within its limits), and delete one. A style SHALL remember the reference it came from (when it still exists) and its name, and SHALL be kept when the reference is deleted. An unknown style SHALL be `404` and an invalid one `422` naming the field.

#### Scenario: Edit
- **WHEN** the accent of a style is replaced by a colour with no contrast
- **THEN** it is refused with `422` naming `design.accent`

#### Scenario: The reference goes
- **WHEN** a reference is deleted
- **THEN** the styles made from it stay

### Requirement: A piece can be made with a style

Creating a piece SHALL accept `preset`, the identifier of a style. The design of the piece SHALL then start as the style's (and be the user's to edit), the fixed rule SHALL take the style's parts in turn (the ones the level supports; all the usual ones when the list is empty) and cut the sections in pieces of the style's rhythm. An unknown style SHALL be refused with `422` and nothing created. A piece made without `preset` SHALL be made exactly as before.

#### Scenario: A piece with a style
- **WHEN** a piece is created with a style whose parts are `terminal` and `counter`
- **THEN** its design is the style's and its script uses only those parts

#### Scenario: Unknown style
- **WHEN** a piece is created with a style that does not exist
- **THEN** the answer is `422` and no piece exists

#### Scenario: No style
- **WHEN** a piece is created with no `preset`
- **THEN** it is made as before

### Requirement: A style can be made without a reference

`POST /api/music-motion/presets` SHALL accept a body with `name`, `design`, `rhythm` and `parts` and no `reference_id`, and then SHALL make the style from those fields without reading any reference or calling the assistant. The style SHALL be validated exactly as a replacement of a style is (the design as a whole design, the rhythm within its limits, the catalog parts without repeats and at most 8) and SHALL be refused with `422` naming the field when it does not pass; `name` SHALL be required. The style SHALL be answered with `201`, stored like any other and have no reference. A body with `reference_id` SHALL behave as before; a body with `reference_id` and any of `design`, `rhythm` or `parts` SHALL be refused with `422`.

#### Scenario: From scratch
- **WHEN** a style is posted with a name, a valid design, a rhythm of 6 and the parts `terminal` and `counter`, with no reference
- **THEN** the answer is `201` and the style is listed with those values and no reference

#### Scenario: Invalid
- **WHEN** the posted accent has no contrast with the background
- **THEN** it is refused with `422` naming `design.accent` and nothing is stored

#### Scenario: Missing name
- **WHEN** a style without `reference_id` has no name
- **THEN** it is refused with `422`

#### Scenario: From a reference as before
- **WHEN** a style is posted with a `reference_id` only
- **THEN** it is made from the reference as before

### Requirement: Styles are kept in the database

The server SHALL keep each style as a row of the database; the ones that exist as files SHALL be imported when the server starts (see `data-store`). `GET /api/music-motion/presets` SHALL be a list that can be paged (see `api-listing`): sort by `name` or `created_at` (the order of today by default) and search `q` in the name. The times the renders of this machine took, which the estimate of a piece starts from, SHALL be kept in the database too and imported the same way.

#### Scenario: After the import

- **WHEN** the server starts with styles kept as files
- **THEN** the list answers the same styles and a piece made with one of them is made as before

#### Scenario: The estimate after the import

- **WHEN** a piece is read after the import
- **THEN** its estimate is the one it had with the file of render times
