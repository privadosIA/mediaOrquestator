# Proposal

## Why

A stored motion page can only be changed today by asking the model for a whole new page (it replaces the stored one) or by editing it by hand. To change how a title enters, the model rewrites some 600 to 1500 tokens of HTML and CSS and can alter the paint that was fine. The page already declares its motion as data, in the `motion-timeline` block, which is a few hundred tokens: a change of motion can be a change of that block only.

## What Changes

- `motion.Revise` asks the model for the new content of the timeline block only, from a request in words ("slower entrances", "a bounce on the title"). The model is shown the page and the clip's size and duration; the answer is the JSON of the block and nothing else.
- The answer is checked as a timeline in Go (version, tracks, ids, start, duration, stagger, curves, properties and their ranges, which a test keeps equal to those of the page's script), written into the stored page with every other byte as it was, and the result goes through the same check as a new page (including the browser check that selectors match and that nothing ends after the clip).
- A failing answer is requested again up to 3 times with the problems and the previous timeline; a page without a timeline block is refused without calling the model.
- `internal/ctxpress` is an optional client of the ctxpress compression service. When `CTXPRESS_URL` is set the page can be shown to the model through it with the `animation-timeline` profile (paint left out, timeline kept); when it is not set, fails, compresses nothing or drops the timeline, the model is shown the page.

## Capabilities

### New Capabilities

### Modified Capabilities
- `motion-graphics`: a stored page's motion can be revised without writing the page again.

## Impact

- New: `internal/video/motion/revise.go` (`Revise`, `ExtractTimeline`, `SpliceTimeline`, `ValidateTimeline`), `internal/ctxpress`, a row in `THIRD_PARTY.md`.
- Needs a ctxpress with the `animation-timeline` profile (ctxpress 0.3.0-dev, contract 2.3.0) only if the compression is wanted; nothing depends on it.
- Not done: the entry point (an editor control, an endpoint or a command that calls `Revise` and stores the page), a setting for the service's address in the Settings page (only the environment variables `CTXPRESS_URL` and `CTXPRESS_TOKEN_FILE`), and a measurement on pages written by the generator (the repository holds only the example page of the prompt).
