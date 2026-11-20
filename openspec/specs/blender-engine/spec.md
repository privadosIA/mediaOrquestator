# blender-engine Specification

## Purpose
Lets a video project use scenes rendered by Blender (headless, Cycles) from a .blend file of the project: configuration, injection of texts and colours, one reserved GPU or the CPU, caching and availability. Blender is optional.

## Requirements

### Requirement: A scene can be rendered by Blender

A scene of a video project SHALL accept `blender`, with `file` (a `.blend` file inside the project's sources), optional `inject` (a map of names to text, numbers or colours) and optional `samples`, `device` (`auto`, `gpu` or `cpu`) and `frames` (first and last frame of the animation to use). The scene SHALL then be made from the render instead of a generated picture. The validation SHALL reject `blender` combined with `prompt`-generated sources (`image`, `init_image`, `clip`, `animate`, `motion_graphic`, `motion_part`), in a project that is not video, with a file that is missing or is not a `.blend`, or with an unknown `device`. No diffusion model SHALL be loaded for such a scene.

#### Scenario: A valid scene
- **WHEN** a scene has `blender: {file: turntable.blend, inject: {title: "Hello"}}` and the file exists
- **THEN** the configuration is valid and no diffusion model is needed for that scene

#### Scenario: Exclusive with other sources
- **WHEN** the scene also has `animate: true` or `motion_graphic`
- **THEN** the validation rejects it and names the fields

#### Scenario: Missing file
- **WHEN** `file` does not exist in the project
- **THEN** the validation rejects it and names the file

### Requirement: The render is headless, scriptless on disk and safe

The render SHALL run Blender in the background with auto-execution of scripts of the file disabled. The values to inject SHALL travel in an environment variable as JSON and SHALL never be interpolated into the script text. No Python file SHALL be written or shipped: the script SHALL be passed inline. Rendering SHALL produce a PNG sequence, and ffmpeg SHALL join it into the scene's video at the project's resolution and frame rate.

#### Scenario: Awkward text is injected unchanged
- **WHEN** an injected text contains quotes, backslashes and a newline
- **THEN** the rendered text shows exactly that text and the script is unchanged

#### Scenario: A file with an auto-run script
- **WHEN** the `.blend` has a script set to run on load
- **THEN** it does not run

### Requirement: The video lasts as long as the scene

The scene's video SHALL last as long as its narration, or its `silence_seconds` for a silent scene, at the project's frame rate. When the animation of the file is shorter than the scene, its last frame SHALL be held; when it is longer, the video SHALL be cut at the scene's length.

#### Scenario: Short animation
- **WHEN** a 2 s animation is used in a 5 s scene
- **THEN** the video lasts 5 s and shows the last frame for the final 3 s

### Requirement: The render uses Cycles on one reserved GPU

The render SHALL use the Cycles engine only. With `device: auto` it SHALL use OptiX on one NVIDIA GPU taken from the run's GPU list and pinned with `CUDA_VISIBLE_DEVICES`, so that every other GPU is not touched; if the run has no GPU it SHALL use the CPU. With `device: cpu` it SHALL never use a GPU, and with `device: gpu` it SHALL fail, naming the reason, when no GPU is available. The GPU SHALL be held by the render the way `sd` holds it, so two processes never share it. The Intel iGPU SHALL NOT be used.

#### Scenario: Two cards
- **WHEN** a run lists GPUs 0 and 1 and a Blender scene renders on 1
- **THEN** only GPU 1 is visible to the render and GPU 0 stays free for another scene

#### Scenario: No GPU
- **WHEN** `device: auto` and the run lists no GPU
- **THEN** the scene renders on the CPU and the log says so

#### Scenario: GPU demanded and absent
- **WHEN** `device: gpu` and no GPU is available
- **THEN** the scene fails with a message saying no GPU is available

### Requirement: The result is cached

The scene's clip SHALL be reused when the content of the `.blend` file, the injected values, `samples`, `frames`, the resolution, the frame rate, the frame count and Blender's version are all unchanged, and SHALL be rendered again when any of them changes. The device (GPU or CPU) SHALL NOT be part of the key.

#### Scenario: Unchanged
- **WHEN** a run is repeated with nothing changed
- **THEN** Blender is not started

#### Scenario: A value changes
- **WHEN** an injected text changes
- **THEN** the scene is rendered again

### Requirement: Blender is optional

When Blender is not installed or is older than 4.2, the engine SHALL be reported as unavailable with the reason, and a project with a Blender scene SHALL be refused before anything runs, naming the reason and how to install Blender. Every other feature SHALL work without it.

#### Scenario: Not installed
- **WHEN** Blender is missing and a project has a `blender` scene
- **THEN** the run is refused with "Blender is not installed" and no scene is processed

#### Scenario: Not needed
- **WHEN** Blender is missing and no scene uses it
- **THEN** everything works as before
