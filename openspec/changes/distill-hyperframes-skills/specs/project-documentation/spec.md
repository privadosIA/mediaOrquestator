# Spec Delta

## ADDED Requirements

### Requirement: The repository carries a skill for the agents that make a music video

`.claude/skills/music-video/SKILL.md` SHALL describe, for an agent driving this server, how to make an animated music video end to end with the HTTP API: the prerequisites and the features to check in the catalog, the steps in order with their routes and bodies (the piece with its song, the words heard, the reference, the proposal of design and script, the modes and the contrast to check, the scenes written and reviewed, the retry of the failed ones only, the approval, the piece review, the render, the contact sheet, the review as a stranger, the fixes), the points where it stops and shows the person what to decide, the craft rules in short with a link to `references/craft.md`, the evidence to leave (a `LEEME.txt` naming every file and id, nothing of the owner's deleted) and what never to do (delete pieces, run `npx` on the server, score without describing). `docs/MUSIC_MODE_MOTION.md` SHALL have a Craft section with the full rules and their origin, and `THIRD_PARTY.md` SHALL credit the HyperFrames references (Apache-2.0, commit) as the source the rules distil. `scripts/check-docs.sh` SHALL verify that every route the skill names exists in `openapi.yaml`.

#### Scenario: A route renamed
- **WHEN** a route the skill names is removed from `openapi.yaml`
- **THEN** `check-docs` fails naming the skill and the route

#### Scenario: The craft is one text
- **WHEN** the Craft section of the docs and `references/craft.md` are compared
- **THEN** they list the same rules
