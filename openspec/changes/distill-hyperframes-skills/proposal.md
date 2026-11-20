# Proposal

## Why

With `add-hyperframes-contract` the server speaks HyperFrames' animation contract and can take its blocks. What HyperFrames also has, and we do not, is the **craft** written down: the skills its agents read before making a film (`skills/hyperframes/references/`: `story.md`, `motion.md`, `film.md` "On music", `types/music.md`, `captions.md`, `review.md`, `look.md`), measured frame by frame on shipped films and licensed Apache-2.0. Our three runs on `camioneta` showed that once the layout rules are enforced, what is left is the **metaphor and the rhythm**: generic shapes, every scene entering the same way, no "home" stage the chorus returns to, cuts that do not follow the energy, a reviewer that scores instead of describing. Those are exactly the things the references cover, and today nothing of them reaches the planner, the script proposer, the scene writer or the reviewer.

A second gap is on the operator's side: the CLI that has been driving the server (making a piece, hearing the words, proposing, writing scenes, reviewing, rendering, leaving evidence in the samba folder) has done it from memory and conversation each time. HyperFrames ships that procedure as a skill; we should too, written for our API.

## What Changes

- **The planner** (`assist/plan`): the plan gains a storyline: `WORLD=` (where it happens and what it looks like, one line), `DEVICE=` (one of the storytelling devices adapted to a music video: type carries the argument; a character does the work; one object across the song; life before and after; a number lands; the result first; a question answered by the song) and `HOOK=` (the first two seconds, exactly). `CONCEPT` stays as the sentence that joins them. The plan is checked as before; the fallback writes a device by rule.
- **The script proposer**: the user message carries, per section, its mean loudness and its energy rank from the song map; the instruction says cut density follows energy (long pieces in the quiet sections, one piece per two to four bars in the loud ones, one held picture on a breakdown), the chorus returns to one **home** part and escalates on each return (same part, stronger parameters or one more extra), the ending rhymes with the opening, a piece longer than 8 s carries a change inside it (an `animate`, a `with` that enters mid-piece, or a layer item), and at most a third of the seams are hard cuts (the rest take `transition_seconds` 0.3–0.6). The fixed rule applies the energy rule and the home part.
- **The scene writer**: the instruction gains the measured motion craft with its numbers: a landed title is dead still; entrances vary within a video (per-word blur-in 0.13 s at 0.067 s apart; per-word grey to ink; the whole line blurring in 8 → 0 px; per letter for a name; a word rolling through options) and never the same fade-and-slide twice in a row; holds of 0.5 s + 0.3 s a word (a one-word line 0.4 s); exits 40–50 % shorter than entrances, with blur-out; the next element starts when the previous is 30–50 % done; ease by role (`expo.out`/`power4.out` arrivals, `power2.inOut` floods and camera, `power3.in` exits); a move that lands on a hit starts 40–190 ms early and the hit itself is a `set`, never eased; hero words 60–90 % of the width against quiet lines at 2.5–4 % of the height; and the list of **defaults that look generated** (breathing, floating or pulsing loops as the motion of a title, a title that keeps sliding or scaling after it lands, a slow creeping scale, bounce on text, every element fading and sliding the same way, a waveform or spectrum bar as the main picture, the same treatment for more than 6 s) as things the reviewer will mark. One metaphor per scene, chosen from the **world** and **device** of the plan, so that the scenes of a video belong to one film.
- **The reviewer**: before scoring, it describes each of the four frames in one sentence as a stranger would, naming every object; an object that would need explaining ("those bars are mirrors") is a fault of the metaphor criterion; its notes follow `t · where · problem → exact change`; the piece review adds the "pictures alone" question (with the words covered, do the frames still tell the story?) and the beat sheet check (does each frame cause the next?). Scores and thresholds stay (the server needs them for `min`/`attempts`).
- **The house verse**: long lines are shown in groups by energy (2–3 words in a loud section, 3–5 in a normal one, 4–6 in a calm one), one group at a time, a group opening 0.1–0.3 s before its first word and closing with a hard kill after its last; the keyword's group is the one that carries the accent. `verse_style` keeps its looks.
- **A skill for the agents that drive the server**: `.claude/skills/music-video/SKILL.md` in this repository (and a copy the dashboards repository can take), with the end-to-end procedure against our API (song → piece with `transcribe`, `separate`, `reference` → `propose all` → check the mode and the contrast → scenes with review → `only: "failed"` → approve → piece review → render → contact sheet → review as a stranger → fixes with `PATCH words`, a scene edit or a brief), the stop points where the person decides, the craft rules above in short form, and what to leave as evidence (the `LEEME.txt` the owner has been asking for).
- **Docs and credit**: a "Craft" section in `docs/MUSIC_MODE_MOTION.md` with the rules and their origin; `THIRD_PARTY.md` credits the HyperFrames references (Apache-2.0) as the source the prompts and the skill distil, with the commit.

## Capabilities

### New Capabilities
- (none)

### Modified Capabilities
- `prompt-assistant`: ADDED the plan of an animated music video carries a storyline.
- `music-motion`: MODIFIED the script proposal (energy, home part, rhyme, change inside long pieces, seams) and the two text layers (grouped verse).
- `motion-scene-parts`: ADDED the writer's craft rules and the generated-look list.
- `motion-scene-review`: MODIFIED the grading (describe first, note format, pictures alone).
- `project-documentation`: ADDED the music-video skill for the agents.

## Impact

- Prompt and template text in `assist/plan.go`, `propose_script.go` (and the fixed rule), `scene_draft.go`/`rubric.go`, `review.go`, `page.go` (verse groups); the songmap already has loudness per 0.25 s. No new routes; `openapi.yaml` gains the plan's three fields.
- Measured before archiving on the same piece and model as the last runs (`camioneta`, `gemma4:31b`, same review settings): the change has to beat the floor the docs record (mean 5.1, 4 fails, 11 of 15 accepted). If an external provider is configured by then, a second column with it.
- The skill is text; it is kept in step with the API by `check-docs` (the routes it names must exist in `openapi.yaml`).
