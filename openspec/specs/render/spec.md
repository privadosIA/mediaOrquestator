# render Specification

## Purpose
Defines how a scene's prompt becomes a rendered still image by driving the stable-diffusion.cpp (`sd`) binary.

## Requirements

### Requirement: Run renders a scene's prompt to a PNG using the configured defaults

`render.Run` SHALL compose the positive prompt and negative prompt from both the scene-level and defaults-level fields before invoking `sd`, and SHALL pass the composed strings to `BuildArgs` as the `prompt` and `negativePrompt` arguments.

**Positive prompt composition.** The effective positive prompt passed to `sd` SHALL be:

- `defaults.SubjectPrompt + " " + scene.Prompt` when `defaults.SubjectPrompt` is non-empty;
- `scene.Prompt` alone when `defaults.SubjectPrompt` is empty or unset.

**Negative prompt composition.** The effective negative prompt passed to `sd` SHALL be:

- `scene.NegativePrompt + " " + defaults.NegativePrompt` when both are non-empty;
- `scene.NegativePrompt` alone when `defaults.NegativePrompt` is empty or unset;
- `defaults.NegativePrompt` alone when `scene.NegativePrompt` is empty or unset;
- the empty string when both are empty or unset (in which case `BuildArgs` SHALL omit the `--negative-prompt` flag, matching current behaviour).

All other `BuildArgs` parameters (model path, resolution, sampler, steps, seed, output path) SHALL be resolved exactly as in the existing `add-script-to-video-pipeline` spec.

#### Scenario: successful render

- **WHEN** `Run` succeeds
- **THEN** `sd` was invoked with the composed positive prompt (subject_prompt + scene prompt, per the composition rules above)
- **AND** with the configured model, sampler, and step count
- **AND** with the configured resolution split into separate width and height flags
- **AND** with the given output path

#### Scenario: subject_prompt is prepended to the scene prompt

WHEN `defaults.SubjectPrompt` is `"A cinematic wide shot of"` and `scene.Prompt` is `"a lighthouse at dusk"`,
THEN `render.Run` SHALL invoke `sd` with the positive prompt argument `"A cinematic wide shot of a lighthouse at dusk"`.

#### Scenario: global negative_prompt is concatenated after the scene negative prompt

WHEN `scene.NegativePrompt` is `"blurry, low quality"` and `defaults.NegativePrompt` is `"watermark, text"`,
THEN `render.Run` SHALL invoke `sd` with the negative prompt argument `"blurry, low quality watermark, text"`.

#### Scenario: both global fields are absent — invocation is identical to today

WHEN `defaults.SubjectPrompt` is empty and `defaults.NegativePrompt` is empty,
THEN `render.Run` SHALL pass `scene.Prompt` as the positive prompt and `scene.NegativePrompt` (or the empty string) as the negative prompt, producing a `BuildArgs` invocation byte-identical to the one produced by the current `add-script-to-video-pipeline` implementation for the same scene and defaults.

### Requirement: An absent seed resolves to sd's random-seed sentinel
`Run` SHALL pass `-1` as the seed when the scene's seed is unset.

#### Scenario: scene has no seed
- **WHEN** the scene's seed is unset
- **THEN** `Run` invokes `sd` with seed `-1`

### Requirement: Run applies GPU environment overrides only to the sd invocation
`Run` SHALL apply any environment overrides passed to it (such as `CUDA_VISIBLE_DEVICES`) to the `sd` subprocess invocation only.

#### Scenario: --gpu sets CUDA_VISIBLE_DEVICES
- **WHEN** `Run` is called with `env` containing `CUDA_VISIBLE_DEVICES=<id>`
- **THEN** that override is applied to the `sd` subprocess invocation

### Requirement: Run rejects a malformed resolution before invoking sd
`Run` SHALL return an error, without invoking the Runner, when the configured resolution is not in `WIDTHxHEIGHT` form.

#### Scenario: resolution missing the "x" separator
- **WHEN** `defaults.resolution` is not in `WIDTHxHEIGHT` form
- **THEN** `Run` returns a non-nil error
- **AND** the Runner is never invoked

### Requirement: Variants render as independent, deterministic sd invocations

When `scene.Variants` is N (N ≥ 1, default 1), the render phase SHALL invoke `sd` exactly N times **sequentially** (never in parallel, to bound VRAM on the render GPU), producing one output PNG per variant at a distinct path.

**Seed selection.**

- **Explicit seed present** (`scene.Seed` is a non-negative integer): variant *i* (0-indexed, i ∈ [0, N−1]) SHALL use seed `scene.Seed + i`. This is fully deterministic and cacheable.
- **No explicit seed** (`scene.Seed` is absent or the sentinel −1): the orchestrator SHALL generate a random 32-bit seed for each variant *before* calling `render.Run` (or an equivalent per-variant entry point). The chosen seed SHALL be passed to `BuildArgs` as the `seed` argument (replacing the sentinel −1) and SHALL be reported back to the caller as the **effective seed** for that variant. The orchestrator is responsible for recording each effective seed in `manifest.json` and including it in the `ImageHash` cache key. `sd`'s internal random draw (triggered by −1) SHALL NOT be relied upon for reproducibility.

**Output paths.** The caller (orchestrator) SHALL supply a distinct `outPath` for each variant invocation. The render package itself SHALL NOT derive or mutate the output path; it SHALL write the PNG to exactly the `outPath` it is given.

**Sequentiality.** The N invocations SHALL complete one after another. No goroutine, channel, or `sync.WaitGroup` SHALL be used to overlap `sd` processes within a single scene's variant loop.

#### Scenario: N=3 with explicit seed 100 produces seeds 100, 101, 102

WHEN `scene.Variants` is 3 and `scene.Seed` is 100,
THEN the render phase SHALL invoke `sd` three times sequentially with seed arguments 100, 101, and 102 respectively, and each invocation SHALL write to a distinct output path supplied by the caller.

#### Scenario: N=2 without explicit seed produces two distinct recorded seeds

WHEN `scene.Variants` is 2 and `scene.Seed` is absent (sentinel −1),
THEN the orchestrator SHALL generate two random 32-bit seeds (S₁, S₂) before the render loop, pass S₁ to the first `sd` invocation and S₂ to the second, and report both S₁ and S₂ as the effective seeds for the export phase to record in `manifest.json`. The two seeds SHALL be distinct with probability 1 − 2⁻³² (collision is theoretically possible but not handled specially).

#### Scenario: N=1 with explicit seed behaves identically to today's single render

WHEN `scene.Variants` is 1 (or unset, defaulting to 1) and `scene.Seed` is 42,
THEN the render phase SHALL invoke `sd` exactly once with seed 42, producing a single PNG at the caller-supplied `outPath`. The `BuildArgs` invocation SHALL be byte-identical to the one produced by the current `add-script-to-video-pipeline` implementation for the same scene, defaults, and output path.

### Requirement: Render drives sd with a model set when one is configured

When `defaults.model_set` is set, `render` SHALL invoke `sd` with `--diffusion-model`, `--llm` and `--vae` carrying the three configured paths, and SHALL NOT pass `-m`. When `cfg_scale` is set it SHALL also pass `--cfg-scale` with that value. All other arguments (prompts, seed, sampler, steps, resolution, output) SHALL be resolved as for a single checkpoint. When no model set is configured the invocation SHALL be byte-identical to today's.

#### Scenario: model set replaces -m
- **WHEN** a scene renders with `defaults.model_set` set and `cfg_scale: 1`
- **THEN** `sd` receives `--diffusion-model`, `--llm`, `--vae` and `--cfg-scale 1` with the configured values
- **AND** `sd` does not receive `-m`

#### Scenario: no cfg_scale configured
- **WHEN** `defaults.model_set` has no `cfg_scale`
- **THEN** `sd` does not receive `--cfg-scale`

#### Scenario: single checkpoint is unchanged
- **WHEN** `defaults.model_set` is unset
- **THEN** the `sd` arguments are identical to the current `-m` invocation

### Requirement: The model set is part of the image identity

The image cache key and the export manifest entry SHALL include the model set (its three paths and `cfg_scale`) when it is configured, and SHALL be unchanged when it is not, so existing caches stay valid. Changing any model-set field SHALL invalidate cached images.

#### Scenario: changing the vae regenerates the image
- **WHEN** a project's cached scene was rendered with one `vae` and the config now names another
- **THEN** the render phase treats the cached image as stale and renders again

#### Scenario: existing caches survive
- **WHEN** a project has no `model_set`
- **THEN** its image hashes equal those computed before this change

### Requirement: A scene with its own picture does not render

A scene with `image` SHALL NOT invoke the diffusion program. Its `render` phase SHALL fit the picture and report the png as artifact, and a project whose scenes all have a picture SHALL NOT require the diffusion program on the PATH.

#### Scenario: No sd call
- **WHEN** a scene with `image` runs
- **THEN** only ffmpeg runs for its render phase

#### Scenario: Preflight
- **WHEN** every scene has a picture and `sd` is missing
- **THEN** the run starts

#### Scenario: Mixed project
- **WHEN** some scenes have a picture and others a prompt
- **THEN** `sd` is required and only the prompt scenes call it

### Requirement: The starting picture is fitted and given to the diffusion program

For a scene with `init_image`, the render phase SHALL first fit the picture to the project resolution (scaled to cover, centre-cropped) into `tmp/scene_<id>.init.png`, then run `sd` as usual with `-i` that file and `--strength` the resolved strength. Every variant SHALL use the same fitted picture. A project with an `init_image` SHALL require ffmpeg in every mode.

#### Scenario: Arguments
- **WHEN** a scene with `init_image: photo.jpg` and `strength: 0.4` renders
- **THEN** ffmpeg fits the picture and `sd` receives `-i tmp/scene_<id>.init.png --strength 0.4`

#### Scenario: Default strength
- **WHEN** `strength` is not set
- **THEN** `sd` receives `--strength 0.6`

#### Scenario: Variants
- **WHEN** a scene has three variants
- **THEN** each `sd` call receives the same `-i` and a different seed

#### Scenario: Without a starting picture
- **WHEN** a scene has no `init_image`
- **THEN** the `sd` arguments are exactly as before

#### Scenario: Preflight in images mode
- **WHEN** an images project has an `init_image` and ffmpeg is missing
- **THEN** the run is refused naming it

### Requirement: A scene renders with its own resolution, steps and cfg scale

The render phase SHALL use the scene's effective render settings: its `render.resolution`, `render.steps` and `render.cfg_scale` when set, otherwise the project's. A scene `cfg_scale` SHALL replace the model set's, and with a single checkpoint SHALL add one `--cfg-scale` to the invocation only when the scene sets it. The image identity (cache key) SHALL use the effective values, so changing a scene's setting regenerates that scene only; a project with no scene `render` SHALL produce the same invocations and the same cache keys as before. In images mode the manifest SHALL record each scene's effective resolution. A starting picture (`init_image`) and a scene's own picture (`image`) SHALL be fitted to the scene's effective resolution.

#### Scenario: different resolutions in one run
- **WHEN** scene A sets `render.resolution: 1280x704` and scene B sets nothing under a project at 1024x576
- **THEN** the invocation for A carries 1280 and 704 and the one for B carries 1024 and 576

#### Scenario: only the changed scene regenerates
- **WHEN** a project has been rendered and one scene's `render.steps` changes
- **THEN** only that scene's image is regenerated

#### Scenario: nothing set
- **WHEN** no scene has a `render` block
- **THEN** the sd arguments and the cache keys are identical to the ones before this feature

#### Scenario: cfg with a single checkpoint
- **WHEN** a scene sets `render.cfg_scale: 3` and the project uses a single checkpoint
- **THEN** its invocation carries `--cfg-scale 3` and the scenes without it carry none

### Requirement: A picture that does not fit the card is made smaller

When the diffusion program fails because the card ran out of memory, the render SHALL make the picture again at about three quarters of the pixels (the same shape, in multiples of 64), up to twice, never below 512x512 worth of pixels, and SHALL emit a warning naming the size asked for and the size used. The picture SHALL be used at the size it was made at, and the images manifest SHALL record that size. A picture made from the user's own image (image to image) SHALL NOT be retried, nor SHALL any failure that is not about memory; after the retries the failure SHALL be reported as before.

#### Scenario: One retry is enough
- **WHEN** a 1280x704 render fails for memory and the same render at 1088x640 works
- **THEN** the run succeeds with the picture at 1088x640, a warning and that size in the manifest

#### Scenario: Another failure
- **WHEN** the program fails for another reason
- **THEN** it is not retried and the run fails as before
