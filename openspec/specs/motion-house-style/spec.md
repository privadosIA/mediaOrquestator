# motion-house-style Specification

## Purpose
TBD - created by archiving change add-house-style-and-fonts. Update Purpose after archive.

## Requirements

### Requirement: A design carries a theme that reaches every page of the piece

`design.theme` MAY hold `css` (≤ 20 KB, refused with `422 design.theme.css: …` when it has `@import`, a `url(` that is not `data:`, `expression(`, `<` or
`javascript:`), `assets` (≤ 5 SVG of ≤ 50 KB each, sanitized like the character library) and `hud` (four texts of ≤ 40 characters). Every page of the
piece, for the render and for the preview, SHALL inject the theme's CSS after the page's own stylesheet, SHALL expose `window.__theme = {assets, hud}`
and SHALL fill `{{asset:name}}` slots in fragments with the sanitized SVG. Presets and datasets SHALL carry the theme with the design; a per-piece design
override MAY replace `theme.hud`.

#### Scenario: Theme styles the verse
- **WHEN** the theme's CSS sets `#verse{font-family:var(--display);font-size:9vh}`
- **THEN** the render's pages and the preview's pages draw the verse that way

#### Scenario: Refused CSS
- **WHEN** `PUT /design` carries a theme CSS with `@import url(https://…)`
- **THEN** the answer is `422` and the reason names `design.theme.css`

#### Scenario: An asset in a scene
- **WHEN** the theme has asset `mascot` and a fragment contains `{{asset:mascot}}`
- **THEN** the page has the sanitized SVG at that place

### Requirement: Free fonts are packed and embedded

The catalog SHALL list the font pack (Inter, Space Grotesk, JetBrains Mono, Playfair Display, Bebas Neue, Fraunces) beside the six stack names, with
`font_stacks` and `font_files` (weights, licence). A design that names a packed font SHALL get its `@font-face` embedded as a `data:` URL in every page
of the piece, identical for the render and the preview; the six stack names SHALL keep working, mapped to the pack where one fits. The pack's licence
files SHALL ship with the program and be listed in `THIRD_PARTY.md`.

#### Scenario: A packed font
- **WHEN** the design's `display` is `bebas-neue`
- **THEN** the page head has an `@font-face` for it with a `data:font/woff2` source and `--display` names it first

#### Scenario: Old names
- **WHEN** the design's `body` is `grotesk`
- **THEN** the page embeds Inter and `--body` names Inter first, then the former stack

### Requirement: The HUD and the grain come with the catalog

The catalog SHALL have `hud-chrome` (four corner labels from the theme's `hud` or its params, a thin rule, optional section tag and timecode, in the body
font) and `grain` (seeded noise and slow bokeh, an intensity parameter, deterministic from `t` and `beat`), both for every level; the docs SHALL show them
as layer items spanning the song. With the `theme` feature the script proposal MAY place them on a layer when the design has `hud`.

#### Scenario: Persistent frame
- **WHEN** `hud-chrome` and `grain` are layer items from 0 to the song's duration
- **THEN** every page of the piece draws the labels and the grain over its own content, and two frames at different beats differ in the grain

#### Scenario: Deterministic grain
- **WHEN** the same page is drawn twice at the same `t`
- **THEN** the two frames are identical

### Requirement: Pieces may be joined with a crossfade

`design.transition_seconds` (0–2, default 0) and a script piece's `transition_seconds` (the join into that piece) SHALL be written into the piece's
project as the scenes' crossfade, so the assembled video dissolves between pieces for that long; 0 SHALL keep today's hard cut; the pages SHALL NOT change; each piece SHALL be drawn as long as the fade into the next one more, so the pieces still land on the song.

#### Scenario: A dissolve on the beat
- **WHEN** the design has `transition_seconds: 0.3` and piece 4 has `transition_seconds: 0`
- **THEN** the video dissolves 0.3 s into every piece but the fourth, which cuts hard
