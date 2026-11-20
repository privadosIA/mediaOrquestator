# animate Specification

## Purpose
Turns a scene's rendered still image into a short animated clip with Wan 2.2 by driving `sd` in video mode, and prepares that clip so the clip phase can stretch it over any narration length.

## Requirements

### Requirement: An animated scene gets a Wan clip generated from its rendered image

For a scene with `animate: true` in a video-mode project, the pipeline SHALL run an `animate` phase after `render` and before `clip`. The phase SHALL invoke `sd` in video-generation mode with the rendered PNG as the initial image, the `defaults.animate` diffusion model, text encoder and VAE, the configured resolution, frame count, frame rate, steps and cfg scale, and the motion prompt. The motion prompt SHALL be the scene's `motion_prompt` when set, otherwise the scene's composed positive prompt. The scene's seed SHALL be used so the result is reproducible. The GPU environment overrides given for the render phase SHALL apply to this invocation too. Scenes without `animate` SHALL not run the phase and SHALL produce no events for it.

#### Scenario: animated scene runs the animate phase
- **WHEN** a scene has `animate: true`
- **THEN** after `render` and before `clip`, `sd` is invoked once in video mode with the scene's PNG as the initial image
- **AND** the invocation carries the configured frame count, frame rate, steps, cfg scale, width and height

#### Scenario: motion_prompt overrides the scene prompt
- **WHEN** a scene sets `motion_prompt: "she turns the page"`
- **THEN** the video invocation's prompt is `she turns the page`

#### Scenario: no motion_prompt falls back to the scene prompt
- **WHEN** an animated scene has no `motion_prompt`
- **THEN** the video invocation's prompt is the scene's composed positive prompt

#### Scenario: non-animated scenes are untouched
- **WHEN** a scene has no `animate`
- **THEN** no animate event is emitted and no video invocation is made

### Requirement: The animated clip is stored as a ping-pong loop

`sd` writes video as AVI, WebM or WebP, not MP4. The animate phase SHALL convert its output into one MP4 file per scene that plays the generated frames forward and then in reverse, so that looping it never jumps. The converted file SHALL be the phase's artifact, at a stable path derived from the scene ID, and the intermediate `sd` output SHALL not be referenced by later phases.

#### Scenario: ping-pong file duration
- **WHEN** `sd` produced 33 frames at 16 fps
- **THEN** the artifact plays those frames forward then reversed, 64 frames and 4 seconds long
- **AND** the reversed half omits the frames that equal the forward half's last and first, so repeating the artifact shows no repeated frame at the join

### Requirement: A failed animation fails the scene with a clear error

When the `sd` video invocation or the conversion fails, the pipeline SHALL emit an error event for the `animate` phase and the scene ID, including the captured stderr, and SHALL stop the run as other phase failures do. It SHALL NOT silently fall back to the still image.

#### Scenario: sd exits non-zero
- **WHEN** the video invocation fails
- **THEN** an error event with `phase: "animate"` and the scene ID is emitted
- **AND** the run stops without producing a clip for that scene

### Requirement: Clips are never chained

Each scene SHALL get exactly one `sd` video invocation. The last frame of one clip SHALL NOT be used as the initial image of another invocation.

#### Scenario: longer narration does not trigger a second generation
- **WHEN** an animated scene's narration is longer than the generated clip
- **THEN** the clip phase stretches the existing clip
- **AND** no second video invocation is made for that scene

### Requirement: An optional high-noise expert turns on the two-expert Wan mode

`defaults.animate` MAY name a `high_noise_diffusion_model` and, with it, `high_noise_steps` and `high_noise_cfg_scale`. When the high-noise expert is set, the animate phase SHALL add to the `sd` command `--high-noise-diffusion-model`, `--high-noise-steps`, `--high-noise-cfg-scale`, `--high-noise-sampling-method euler` and `--flow-shift 3.0`, and `diffusion_model` SHALL be the low-noise expert. When it is not set, the `sd` command SHALL be exactly what it was before this capability existed: none of these arguments, and the same defaults.

#### Scenario: two-expert command
- **WHEN** `high_noise_diffusion_model` is `models/video/Wan2.2-I2V-A14B-HighNoise-Q4_K_S.gguf`
- **THEN** the `sd` arguments include `--high-noise-diffusion-model` with that path, `--high-noise-steps 8`, `--high-noise-cfg-scale 3.5`, `--high-noise-sampling-method euler` and `--flow-shift 3.0`

#### Scenario: single-model command is unchanged
- **WHEN** no high-noise expert is set
- **THEN** the `sd` arguments contain no `--high-noise-*` option and no `--flow-shift`, and are identical to those of a project that never had these fields

### Requirement: The two-expert mode has its own defaults

When the high-noise expert is set, unset settings SHALL resolve to 10 `steps`, `cfg_scale` 3.5, `high_noise_steps` 8 and `high_noise_cfg_scale` 3.5; `resolution`, `frames` and `fps` keep their usual defaults. When it is not set, `steps` and `cfg_scale` SHALL keep resolving to 20 and 6. A value the user wrote SHALL always win over a default, and an explicit `cfg_scale` or `high_noise_cfg_scale` of 0 SHALL be kept.

#### Scenario: A14B defaults
- **WHEN** a high-noise expert is set and nothing else about steps or cfg is
- **THEN** the command uses 10 steps, cfg 3.5, 8 high-noise steps and high-noise cfg 3.5

#### Scenario: the 5B defaults stay
- **WHEN** no high-noise expert is set and nothing about steps or cfg is
- **THEN** the command uses 20 steps and cfg 6

#### Scenario: the user wins
- **WHEN** a high-noise expert is set with `steps: 14` and `high_noise_cfg_scale: 0`
- **THEN** the command uses 14 steps and a high-noise cfg of 0

### Requirement: Two-expert settings are validated

The high-noise settings SHALL be refused when they make no sense: `high_noise_steps` or `high_noise_cfg_scale` without a high-noise expert, `high_noise_steps` below 1, and a negative `high_noise_cfg_scale`. When a scene animates, the file named by `high_noise_diffusion_model` SHALL exist, with the same kind of error as for the other model files.

#### Scenario: steps without an expert
- **WHEN** `high_noise_steps` is set and `high_noise_diffusion_model` is not
- **THEN** validation fails saying that the high-noise settings need `high_noise_diffusion_model`

#### Scenario: missing file
- **WHEN** a scene animates and the high-noise expert file does not exist
- **THEN** validation fails naming `defaults.animate.high_noise_diffusion_model` and the path

### Requirement: The animate cache distinguishes the two-expert mode without invalidating existing caches

The animate cache key SHALL include the high-noise expert, its steps and cfg, and the flow shift only when the high-noise expert is set; otherwise the key SHALL be exactly the one computed before this capability. Changing any of those values SHALL regenerate the clip.

#### Scenario: existing projects keep their cache
- **WHEN** a project without a high-noise expert is run after this change
- **THEN** its animate cache key equals the one computed before, so its clips are cache hits

#### Scenario: changing the expert regenerates
- **WHEN** the high-noise expert, `high_noise_steps` or `high_noise_cfg_scale` changes
- **THEN** the clip is generated again

### Requirement: The memory reserved for an animation includes the high-noise expert

The memory reserved for an animated scene SHALL include the size of the high-noise expert file together with the other Wan files, so concurrent processes cannot together exceed the memory budget. An animation whose estimate exceeds the whole budget SHALL run alone, as any oversized process does.

#### Scenario: both experts are counted
- **WHEN** a scene animates with two 8.75 GB experts, a 3.5 GB text encoder and a 0.25 GB VAE
- **THEN** the reservation is the estimate of the sum of the four files

#### Scenario: no expert, no change
- **WHEN** no high-noise expert is set
- **THEN** the reservation is the estimate of the three files, as before

### Requirement: An animated scene uses its own animation settings

The animate phase SHALL use the scene's effective animation settings: its `animation.resolution`, `animation.frames` and `animation.steps` when set, otherwise those of `defaults.animate`. The animated clip's duration, which the clip phase uses to stretch it over the narration, SHALL be computed from that scene's frame count and the project's frame rate. The animate cache key SHALL use the effective values; a project with no scene `animation` SHALL produce the same invocations and the same cache keys as before.

#### Scenario: a scene with more frames
- **WHEN** an animated scene sets `animation.frames: 49` under a project with 33
- **THEN** its invocation carries 49 frames, its clip lasts (2×49−2)/fps seconds, and the other animated scenes still use 33

#### Scenario: the cache follows the scene
- **WHEN** one scene's `animation.steps` changes
- **THEN** only that scene's animation is regenerated

### Requirement: The animate phase can use AnimateDiff instead of Wan

When `defaults.animate.engine` is `animatediff`, an animated scene SHALL still run the `animate` phase between `render` and `clip`, but the phase SHALL invoke `sd` in video-generation mode with the SD 1.5 `checkpoint` as the model, the `motion_module`, the scene's still (fitted as the next requirement says) as the initial image, `--strength`, the scene's seed, the motion prompt chosen as for Wan, the effective resolution, frame count, frame rate, steps and cfg scale, the sampling method `euler` and the scheduler `karras`. It SHALL NOT pass the Wan model, text-encoder, VAE, high-noise or flow-shift arguments. The GPU environment overrides of the render phase SHALL apply too. When `engine` is absent or `wan`, the phase SHALL behave exactly as the Wan requirements say.

#### Scenario: AnimateDiff command
- **WHEN** a scene animates with `engine: animatediff`, checkpoint `models/v1-5-pruned-emaonly.safetensors`, module `models/video/mm_sd15_v3.safetensors` and the engine defaults
- **THEN** `sd` is invoked once with `-M vid_gen`, `--model` and `--motion-module` with those paths, `--strength 0.75`, `--cfg-scale 7`, `--sampling-method euler`, `--scheduler karras`, `-W 512 -H 512`, `--video-frames 16`, `--fps 8` and `--steps 25`
- **AND** the arguments contain no `--diffusion-model`, `--t5xxl`, `--vae`, `--high-noise-*` or `--flow-shift`

#### Scenario: Wan projects are unchanged
- **WHEN** a project has no `engine` in `defaults.animate`
- **THEN** its `sd` arguments are identical to those of a project that never had this option

#### Scenario: the motion prompt is the same as for Wan
- **WHEN** an AnimateDiff scene sets `motion_prompt`
- **THEN** that text is the invocation's prompt, and without it the scene's composed positive prompt is used

### Requirement: The still is cover-cropped to the animation resolution, never stretched

Before invoking `sd` for AnimateDiff, the phase SHALL write a copy of the scene's still scaled so it covers the animation resolution and then centre-cropped to exactly that resolution, under `tmp/`, and SHALL pass that copy as the initial image. The scene's own still SHALL NOT be modified.

#### Scenario: a wide still and a square animation
- **WHEN** a 1280x704 still is animated at 512x512
- **THEN** `sd` receives a 512x512 image made by scaling the still to cover and cropping its centre, not a stretched one
- **AND** the original 1280x704 still is unchanged

### Requirement: The AnimateDiff clip is converted and stretched like any animated clip

The AVI that `sd` writes for AnimateDiff SHALL be converted into the same forward-and-back MP4 at the same stable per-scene path, and its duration SHALL be (2×frames−2)/fps seconds for the scene's effective frames and the engine's frame rate, so the clip phase stretches it over the narration as it does a Wan clip. Each scene SHALL still get exactly one `sd` invocation and clips SHALL NOT be chained.

#### Scenario: 16 frames at 8 fps
- **WHEN** `sd` produced 16 frames at 8 fps
- **THEN** the artifact plays them forward then reversed, 30 frames and 3.75 seconds long

### Requirement: A failed AnimateDiff animation fails the scene

When the AnimateDiff `sd` invocation, the still fitting or the conversion fails, the pipeline SHALL emit an error event for the `animate` phase and the scene ID with the captured stderr and stop the run, without falling back to the still image, as for Wan.

#### Scenario: the checkpoint is not an SD 1.5 model
- **WHEN** `sd` exits non-zero because the checkpoint does not match the motion module
- **THEN** an error event with `phase: "animate"` and the scene ID carries sd's stderr and the run stops

### Requirement: The memory reserved for an AnimateDiff animation is the checkpoint plus the module

For an AnimateDiff project the memory reserved for an animated scene SHALL be the estimate of the sizes of the checkpoint and the motion module, instead of the Wan files, so concurrent processes cannot together exceed the budget.

#### Scenario: SD 1.5 plus the module
- **WHEN** a scene animates with a 4.3 GB checkpoint and a 0.84 GB motion module
- **THEN** the reservation is the estimate of the sum of those two files

### Requirement: MiniMax-H3 can use a LoRA
`defaults.animate.lora` MAY name a LoRA file for `minimax-h3` (it SHALL exist when set). With it, the `sd` command SHALL add `--lora-model-dir` with the file's directory and ` <lora:NAME:1>` (NAME = file name without extension) at the end of the prompt, the default steps SHALL be 4 instead of 20, and the LoRA SHALL be part of the animation cache hash only when set.

#### Scenario: turbo LoRA
- **WHEN** `lora: models/video/turbo.safetensors` and no `steps`
- **THEN** `sd` gets `--steps 4`, `--lora-model-dir models/video` and the prompt ends with `<lora:turbo:1>`

### Requirement: The animate phase can use MiniMax-H3

When `defaults.animate.engine` is `minimax-h3`, an animated scene SHALL still run the `animate` phase between `render` and `clip`, and the phase SHALL invoke `sd` in video-generation mode with `--diffusion-model`, `--vae`, `--audio-vae` and `--llm` from the settings, the scene's still (fitted to the animation resolution as for AnimateDiff) as the reference image `-r`, the scene's seed, the motion prompt chosen as for Wan prefixed with the instruction to keep the reference's subject, the effective resolution, frame count, frame rate (24), steps and cfg scale, and `--diffusion-fa --offload-to-cpu --rng cpu`. It SHALL NOT pass the Wan text-encoder, init-image, high-noise or flow-shift arguments. The GPU environment overrides of the render phase SHALL apply. The audio the model writes SHALL be used only when `model_audio` is on (see "The model's own audio can be kept as ambience"); otherwise the clip is converted like any animated clip and the audio is dropped.

#### Scenario: MiniMax-H3 command
- **WHEN** a scene animates with `engine: minimax-h3` and the engine defaults
- **THEN** `sd` is invoked once with `-M vid_gen`, the four model files, `-r` with the fitted still, `-W 864 -H 480`, `--video-frames 56`, `--fps 24`, `--steps 20`, `--cfg-scale 1`, `--diffusion-fa`, `--offload-to-cpu`, `--mmap` and `--rng cpu`
- **AND** the arguments contain no `--t5xxl`, `-i`, `--high-noise-*` or `--flow-shift`

#### Scenario: Wan projects are unchanged
- **WHEN** a project has no `engine` in `defaults.animate`
- **THEN** its `sd` arguments are identical to those before this engine existed

### Requirement: MiniMax-H3 can use the picture as the first frame

`defaults.animate.first_frame: true` SHALL make the minimax-h3 engine pass the scene's picture to sd as the first frame (`--init-img`) instead of a reference image (`-r`), and SHALL NOT add the reference sentence to the prompt. With a LoRA the default steps SHALL be 8 instead of 4. The option SHALL be part of the animation cache key only when set, and SHALL be invalid for any other engine.

#### Scenario: First-frame arguments
- **WHEN** a clip is made with `first_frame: true`
- **THEN** sd receives `-i <picture>` and no `-r`, and the prompt is the motion prompt alone

#### Scenario: Another engine
- **WHEN** `first_frame` is set with the Wan engine
- **THEN** the project is invalid

### Requirement: The animate phase can use LTX-2.3

When `defaults.animate.engine` is `ltx-2.3`, an animated scene SHALL still run the `animate` phase between `render` and `clip`, and the phase SHALL invoke `sd` in video-generation mode with `--diffusion-model`, `--vae`, `--audio-vae`, `--llm` and `--embeddings-connectors` from the settings, the scene's still (fitted to the animation resolution as for AnimateDiff) as the first frame `-i`, the scene's seed, the motion prompt chosen as for Wan (without any added sentence), the effective resolution, frame count, frame rate (24), steps and cfg scale, `--sampling-method euler`, and `--offload-to-cpu --mmap`. It SHALL NOT pass the Wan text-encoder, high-noise, flow-shift or reference-image (`-r`) arguments, nor a LoRA. The GPU environment overrides of the render phase SHALL apply. The audio the model writes SHALL be used only when `model_audio` is on (see "The model's own audio can be kept as ambience"); otherwise the clip is converted like any animated clip and the audio is dropped.

#### Scenario: LTX-2.3 command
- **WHEN** a scene animates with `engine: ltx-2.3` and the engine defaults
- **THEN** `sd` is invoked once with `-M vid_gen`, the five model files, `-i` with the fitted still, `-W 768 -H 416`, `--video-frames 49`, `--fps 24`, `--steps 8`, `--cfg-scale 1`, `--sampling-method euler`, `--offload-to-cpu` and `--mmap`
- **AND** the arguments contain no `--t5xxl`, `-r`, `--high-noise-*`, `--flow-shift` or `<lora:`

#### Scenario: Other engines are unchanged
- **WHEN** a project uses Wan, AnimateDiff or MiniMax-H3
- **THEN** its `sd` arguments are identical to those before this engine existed

#### Scenario: A failed LTX-2.3 animation
- **WHEN** `sd` exits with an error for an LTX-2.3 scene
- **THEN** the scene fails with the error and its log, as for any engine

### Requirement: The model's own audio can be kept as ambience

When `defaults.animate.model_audio` is true and the engine is `minimax-h3` or `ltx-2.3`, after `sd` the animate phase SHALL write the AVI's audio to a stereo 44100 Hz WAV next to the animation (`tmp/scene_<scene>.anim.ambient.wav`), taking only the forward part (the first `frames / fps` seconds, never the reversed half) with a 0.3 s fade out at its end. The ping-pong MP4 SHALL stay video only. If the AVI has no audio stream the scene SHALL fail with an error that names the scene. Without `model_audio` no WAV SHALL be written and the ffmpeg calls SHALL be those of today.

#### Scenario: Extraction
- **WHEN** an LTX-2.3 scene with 49 frames at 24 fps animates with `model_audio: true`
- **THEN** a second ffmpeg call writes a WAV of about 2.04 s with a fade out ending at its end, and the ping-pong call is unchanged

#### Scenario: Off by default
- **WHEN** `model_audio` is not set
- **THEN** no WAV is written
