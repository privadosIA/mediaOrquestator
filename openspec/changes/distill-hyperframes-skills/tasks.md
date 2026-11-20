# Tasks

Commit messages in Spanish. Every task names how to verify it. Apply after `add-hyperframes-contract` is archived. The sources are HyperFrames' `skills/hyperframes/references/` (`story.md`, `types/music.md`, `film.md` "On music", `motion.md`, `captions.md`, `review.md`), Apache-2.0: read them at one commit and write that commit in `THIRD_PARTY.md` (fill the commit in the same task, never leave a placeholder) with the Apache-2.0 notice if any text is quoted; distil, never paste paragraphs.

## 1. The craft, written once

- [x] 1.1 `docs/MUSIC_MODE_MOTION.md` § "Craft": the rules of design D2–D5 with their numbers, grouped (story: world, device, hook; edit: energy, home part, rhyme, change inside long pieces, seams; motion: titles, entrances, holds, exits, overlap, ease by role, hits, sizes; the generated looks; captions: groups by energy, open and close; review: describe first, note format, pictures alone), each marked "measured by HyperFrames on their films" or "ours". `.claude/skills/music-video/references/craft.md` with the same list (a test compares the two lists). `THIRD_PARTY.md`: the credit with the commit. Verify: `check-docs`; the comparison test.

Commit: `Reglas de oficio del vídeo musical, escritas una vez y acreditadas`

## 2. Planner and script

- [x] 2.1 `assist/plan.go`: `WORLD=`, `DEVICE=` (the seven), `HOOK=` in `planSystem` and the parser; the fallback device with its note; `PlanResult` fields; the piece keeps `world`, `device`, `hook` when made from a plan (the flow sends them in `Choices`); `openapi.yaml`. Verify: `plan_test` with a full answer, an unknown device and a missing hook; the handler test sees the three on the piece.
- [x] 2.2 `propose_script.go`: per-section `loudness` and `energy` in `scriptUser` (from `songmap` loudness over the section's span), the six rules in `scriptSystem`, `"home"` accepted at the top level of the answer (kept in the script as `home`), device and hook in the user message; the fixed rule: lengths by energy rank (quiet 8–15 s, loud one piece per 2–4 bars, breakdown held), one part for every chorus piece. `design.go`'s proposal receives world and device in its user message. Verify: `prompt_test` goldens; the fixed-rule test of the spec's scenario; the validator unchanged (`home` optional).

Commit: `El plan lleva mundo, recurso y gancho; el guion sigue la energía y vuelve a casa en el estribillo`

## 3. Writer, reviewer, verse

- [x] 3.1 `scene_draft.go`: the craft block of D3 in `sceneSystem`; `SceneDraftInput.World|Device|Hook`; the first scene's chosen world kept on the piece by the batch when the piece has none (`world` written by the batch from a `WORLD:` line the writer is asked to put first in the manifest's description; when the line is missing the piece's `world` stays empty and the next block says "choose a world and keep it"); `rubric.go`: the metaphor criterion's clause. Verify: `prompt_test` goldens; a batch test on a piece without a plan shows the second scene's input carrying the first's world.
- [x] 3.2 `review.go`: the four sentences first, the note format, the metaphor cap from the sentences (the word list, the abstract-world exemption from `world`), `review.frames`; the piece review's two questions, `pictures_alone`, `causes_next`, the generated looks and the older scenes in the overall notes. `openapi.yaml`. Verify: tests with a fake vision client (a "grey bars" sentence caps the metaphor at 1; an abstract world does not; the two answers recorded); `TestOpenAPI_*`.
- [x] 3.3 `page.go` (`layerScript`) and `scenepage.go`: the verse groups of D5 (energy rank from the map's loudness per section; groups; open 0.2 s before, kill 0.12 s after; keyword inside; `verse_groups: false` on the design turns it off); deterministic. The craft block tells a scene that owns the verse the same rule. Verify: `page_test`: a nine-word line in the loudest section gives groups of 2–3 with the open/close times; a five-word line unchanged; `verse_groups: false` unchanged; two renders equal.

Commit: `El escritor conoce el oficio y el mundo del vídeo; el revisor describe antes de puntuar; el verso se agrupa por energía`

## 4. The skill

- [x] 4.1 `.claude/skills/music-video/SKILL.md` (frontmatter `name: music-video`, `description`), the sections of design D6 with the exact routes and bodies from `openapi.yaml`, the stop points, the short craft with the link, the evidence rules and the never-do list; `references/craft.md` from 1.1. `scripts/check-docs.sh`: the routes named in the skill exist in `openapi.yaml`. Verify: `check-docs` passes; rename a route in a scratch copy and see it fail.
- [ ] 4.2 Run the skill as written, with the CLI, on a fresh song chosen by the owner (ask which; not `camioneta`, not `reunion`), end to end, stopping where it says; fix the skill where the CLI had to guess. Verify: the run leaves the evidence folder with its `LEEME.txt`, and the skill's text changed only where the run showed a gap (listed in the commit message).
  - Nota 2026-11-16: ejecutada con `sunrise` (balada del amanecer, HeartMuLa) y parada tras la ronda 3 de escenas (10 de 25 listas, el modelo repite los mismos fallos). La skill NO se ha corregido todavía; los huecos están anotados en la carpeta de pruebas del dueño (LEEME.txt de skill-music-video-picor). Sigue abierta.

Commit: `Skill music-video: el procedimiento completo contra la API, con sus paradas y sus pruebas`

## 5. Measure and close

- [x] 5.1 `go vet ./... && go test ./... && go test -tags private ./...`; `check-docs`. Verify: exit 0.
- [ ] 5.2 The piece `camioneta` again, same model (`gemma4:31b`) and review settings as the recorded floor (mean 5.1, 4 fails, 11 of 15 accepted): plan fields set by hand on the piece (world, device, hook from the song), script proposed again with the new rules, all 15 scenes written, approved, piece reviewed, rendered; the column added to the docs' table with the four sentences of two scenes as examples; if an external provider is configured, a second column with it. Verify: the table has the new column(s) and a sentence saying whether the floor was beaten and on what, and the time per scene next to the floor's (a better score that doubles the time is reported as such).
