# Spec Delta

## ADDED Requirements

### Requirement: The plan of an animated music video carries a storyline

The plan written from one sentence SHALL also carry `world` (where the video happens and what it looks like, one line), `device` (one of: `type carries the argument`, `a character does the work`, `one object across the song`, `life before and after`, `a number lands`, `the result first`, `a question the song answers`) and `hook` (what the first two seconds show, one line), in English, next to the concept. A missing or unknown device SHALL become `type carries the argument` with a note. The three SHALL be kept with a piece made from the plan and given to the design proposal (world and device), the script proposal (device and hook) and the scene writer (world and device, the hook for the first piece), so that the scenes of a video belong to one film.

#### Scenario: A plan with a device
- **WHEN** the request is "a night drive through a city that forgets you"
- **THEN** the plan has a world (the city at night, one line), a device from the list and a hook, and the piece made from it keeps the three

#### Scenario: An unknown device
- **WHEN** the model answers a device that is not in the list
- **THEN** the plan says `type carries the argument` and its note says the model's device was not one offered
