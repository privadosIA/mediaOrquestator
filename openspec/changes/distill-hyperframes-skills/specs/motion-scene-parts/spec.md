# Spec Delta

## ADDED Requirements

### Requirement: The scene writer is told the craft and the video's world

The instruction given to the writer of a scene SHALL carry a craft block with numbers: a landed title stays still; entrances vary within a video (per word with a blur of 6 px to 0 and opacity 0 to 1 in 0.13 s at 0.067 s apart; per word from light grey to ink; the whole line blurring in from 8 px in 0.13–0.2 s; per letter for a name at 0.1 s apart; one word rolling through options) and the same entrance is never used twice in a row; a line holds 0.5 s plus 0.3 s a word (a one- or two-word line 0.4 s); exits are 40–50 % shorter than entrances and leave with a blur-out; the next element starts when the previous is 30–50 % done; staggers of a frame between letters, 60–120 ms between words; arrivals ease out on a long tail (`expo.out`, `power4.out`), floods and camera moves ease both ways (`power2.inOut`), exits accelerate (`power3.in`), nothing linear but a steady drift; a move that lands on a hit starts 40–190 ms early, the hit itself is set without easing, a reaction starts 0–45 ms after; the hero word fills 60–90 % of the width and quiet lines are 2.5–4 % of the height; and the list of looks that read as generated (a breathing, floating or pulsing loop as a title's motion; a title that keeps sliding or scaling after it lands; a slow creeping scale; bounce on text; every element fading and sliding the same way; a waveform or spectrum as the main picture; one treatment for more than 6 s) as things the review marks. The writer's input SHALL carry the plan's world and device when the piece has them, and the instruction SHALL ask for one metaphor that belongs to them; without them, the writer SHALL choose a world for the first scene of the piece and the batch SHALL keep it for the next scenes. The rubric's metaphor criterion SHALL also ask whether the metaphor belongs to the video's world.

#### Scenario: The craft reaches the writer
- **WHEN** a scene is asked for
- **THEN** the writer's instruction contains the hold rule (0.5 s plus 0.3 s a word) and the generated-look list

#### Scenario: One world
- **WHEN** a piece was made from a plan with world "a night road, headlights and dust" and device "one object across the song"
- **THEN** every scene's input carries both and the rubric asks whether the metaphor belongs to that world

#### Scenario: No plan
- **WHEN** a piece has no world and its first scene is written
- **THEN** the next scene's input carries the world the first scene chose
