# blender-engine Specification

## Purpose
Lets a video project use scenes rendered by Blender (headless, Cycles) from a .blend file of the project, or built from a JSON scene description (kept as a .blend): configuration, injection of texts and colours, one reserved GPU or the CPU, caching and availability. Blender is optional.

## Requirements

### Requirement: A scene can be rendered by Blender

A scene of a video project SHALL accept `blender`, with exactly one of `file` (a `.blend` file inside the project's sources) or `build` (a scene description, a `.scene.json` file inside the project's sources, given as a relative path), optional `inject` (a map of names to text, numbers or colours; only with `file`) and optional `samples`, `device` (`auto`, `gpu` or `cpu`) and `frames` (first and last frame of the animation to use; only with `file`). The scene SHALL then be made from the render instead of a generated picture. The validation SHALL reject `blender` combined with `prompt`-generated sources (`image`, `init_image`, `clip`, `animate`, `motion_graphic`, `motion_part`), in a project that is not video, with both or neither of `file` and `build`, with `inject` or `frames` alongside `build`, with a file that is missing or is not a `.blend` (for `file`) or not a `.scene.json` (for `build`), with a `file` or `build` that is an absolute path, contains `..` or resolves outside the project's sources, or with an unknown `device`. No diffusion model SHALL be loaded for such a scene.

#### Scenario: A valid scene
- **WHEN** a scene has `blender: {file: turntable.blend, inject: {title: "Hello"}}` and the file exists
- **THEN** the configuration is valid and no diffusion model is needed for that scene

#### Scenario: A built scene
- **WHEN** a scene has `blender: {build: titulo.scene.json, samples: 32}` and the description is valid
- **THEN** the configuration is valid and no diffusion model is needed for that scene

#### Scenario: Exclusive with other sources
- **WHEN** the scene also has `animate: true` or `motion_graphic`
- **THEN** the validation rejects it and names the fields

#### Scenario: Both file and build
- **WHEN** a scene has `file` and `build`, or `build` with `inject`
- **THEN** the validation rejects it and names the fields

#### Scenario: Missing file
- **WHEN** `file` or `build` does not exist in the project
- **THEN** the validation rejects it and names the file

### Requirement: The render is headless, scriptless on disk and safe

The render SHALL run Blender in the background with auto-execution of scripts of the file disabled. The values to inject SHALL travel in an environment variable as JSON and SHALL never be interpolated into the script text; a scene description SHALL be passed as the path of the file, read by the script as JSON data, and none of its values SHALL be evaluated, formatted into code or used to look up a Python name except through fixed tables of allowed values. No Python file SHALL be written or shipped: the script SHALL be passed inline. Rendering SHALL produce a PNG sequence, and ffmpeg SHALL join it into the scene's video at the project's resolution and frame rate.

#### Scenario: Awkward text is injected unchanged
- **WHEN** an injected text contains quotes, backslashes and a newline
- **THEN** the rendered text shows exactly that text and the script is unchanged

#### Scenario: Awkward text in a description
- **WHEN** a text object of a description has quotes, backslashes, a newline and `__import__('os')`
- **THEN** the rendered text shows exactly those characters and nothing is executed

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

The scene's clip SHALL be reused when the content of the `.blend` file (for `file`) or of the description resolved with the project's tempo and design colours, and the font file it uses (for `build`), the injected values, `samples`, `frames`, the resolution, the frame rate, the frame count and Blender's version are all unchanged, and SHALL be rendered again when any of them changes or, for a built scene, when its kept `.blend` is missing. The device (GPU or CPU) SHALL NOT be part of the key.

#### Scenario: Unchanged
- **WHEN** a run is repeated with nothing changed
- **THEN** Blender is not started

#### Scenario: A value changes
- **WHEN** an injected text changes
- **THEN** the scene is rendered again

#### Scenario: The design's accent changes
- **WHEN** a built scene uses `accent` and the project's design changes its accent colour
- **THEN** the scene is rendered again

#### Scenario: The tempo changes
- **WHEN** a built scene uses `beat` and the project's tempo changes
- **THEN** the scene is rendered again

#### Scenario: The design's font changes
- **WHEN** a built scene uses `font: "display"` and the design's display font changes
- **THEN** the scene is rendered again

### Requirement: Blender is optional

When Blender is not installed or is older than 4.2, the engine SHALL be reported as unavailable with the reason, and a project with a Blender scene SHALL be refused before anything runs, naming the reason and how to install Blender. Every other feature SHALL work without it.

#### Scenario: Not installed
- **WHEN** Blender is missing and a project has a `blender` scene
- **THEN** the run is refused with "Blender is not installed" and no scene is processed

#### Scenario: Not needed
- **WHEN** Blender is missing and no scene uses it
- **THEN** everything works as before

### Requirement: A scene can be built from a description

A `.scene.json` description (contract version 1) SHALL describe a world colour and strength, a camera (lens and keyframed `location` with `target` or `rotation`), up to 8 lights (`sun`, `area`, `point`, with `power`, `color`, `size` for area, and `location` with `target` or `rotation`), up to 32 named materials (`color`, `roughness`, `metallic`, `emission`), and up to 64 objects of type `text`, `cube`, `sphere`, `cylinder`, `cone`, `torus`, `plane` or `backdrop` (at most one; a floor curving into a back wall with `size` and `curve`, `curve` within [0.1, size/2]), each with an `id`, a `size`, a `material` and keyframes. A key SHALL carry exactly one of `t` (seconds from the start of the scene) or `beat` (a number ≥ 0, fractions allowed, converted to seconds with the project's tempo, `defaults.motion.bpm`), and any of `location`, `rotation` (degrees), `scale` (a number or three) and `emission`, and an optional `ease` (`linear`, `inout`, `in`, `out`, `back`, `bounce`, `elastic`, `step`; `inout` when absent) for the interval that ends at it (the ease of a channel's first key is ignored), where `step` holds the previous value until the key's frame and jumps to the key's value on it; only the channels a key gives SHALL be keyed, and a key at the scene's length (`t` equal to it, or the `beat` that falls on it) SHALL fall on the scene's last frame. A text object SHALL take `text` (up to 120 characters), `extrude`, `bevel`, `align` and an optional `font`: `display` or `body` (the design's display font at weight 700 when the pack has it, else 400, or the design's body font at 400, taken from the server's font pack and converted to a format Blender reads), or a `.ttf` or `.otf` in the sources (a relative path under the same rule as `build`); Blender's built-in font when absent. A colour SHALL be `#rrggbb` or one of `bg`, `fg`, `accent`, taken from the project's design. The server SHALL check the description before starting Blender and SHALL refuse it, naming the JSON path of the fault, when its version is not 1, it is larger than 256 KiB, a limit above is exceeded, an object has more than 400 keys, a key has both or neither of `t` and `beat`, a `beat` is used in a project without a tempo, a `t` or a converted `beat` is outside the scene's length (the message giving both the beat and its second), a coordinate is outside ±1000, an `id` is not `^[a-z][a-z0-9_-]{0,31}$` or is repeated, or a `material` or `font` named does not exist. Blender SHALL start from an empty scene, build what the description says, set the scene's frame rate to the project's, and render as a `file` scene renders. When a built scene does not give `samples`, it SHALL use the default for built scenes recorded in the docs from the measure of this change.

#### Scenario: The example
- **WHEN** the project `testdata/blender-build` is rendered at 1280×720 and 30 fps
- **THEN** the clip lasts 5 s, the title in the design's display font rises from below with an overshoot by beat 1.2, the camera moves toward it, there is no horizon line behind it, and the accent sphere is brightest on the frame of each beat from 1 to 9 and not on the frame before

#### Scenario: A hit does not anticipate
- **WHEN** an object has a key `{beat: 4, emission: 4, ease: "step"}` after a key with emission 0, at 120 bpm and 30 fps
- **THEN** its emission is 0 on frame 60 and 4 on frame 61

#### Scenario: Beats without a tempo
- **WHEN** a description uses `beat` in a project with no `defaults.motion.bpm`
- **THEN** the scene is refused before Blender starts and the message names the first key that uses it

#### Scenario: A beat beyond the scene
- **WHEN** a key has `beat: 12` at 120 bpm in a 5 s scene
- **THEN** the scene is refused and the message names the key, beat 12 and 6 s

#### Scenario: A key at the end
- **WHEN** a key has `t: 5` (or `beat: 10` at 120 bpm) in a 5 s scene at 30 fps (150 frames)
- **THEN** the scene is accepted and that key lands on frame 150

#### Scenario: A path outside the sources
- **WHEN** `build` or a text object's `font` is `../x.scene.json`, an absolute path, or a symbolic link to a file outside the sources
- **THEN** the scene is refused before Blender starts and the message names the field

#### Scenario: A key out of the scene
- **WHEN** a key has `t: 7` in a 5 s scene
- **THEN** the scene is refused before Blender starts and the message names `objects[0].keys[3].t`

#### Scenario: An unknown material
- **WHEN** an object names a material the description does not define
- **THEN** the scene is refused naming the object and the material

#### Scenario: The design's palette
- **WHEN** the project's design has accent `#ff3b30` and a material's colour is `accent`
- **THEN** that material renders in that colour

#### Scenario: The design's font
- **WHEN** the design's display font is Playfair Display and a text has `font: "display"`
- **THEN** the text renders in Playfair Display 700 and the kept `.blend` holds that font packed

### Requirement: The built scene is kept as a .blend

Before rendering, a built scene SHALL be saved, with its fonts packed, as `<scene-id>.blend` next to the scene's clip in the project's output, and SHALL be downloadable through the project's files. The file SHALL open in Blender 5.3 or newer (the version that saves it) on another machine with the same objects, materials, camera, lights and keyframes, and SHALL carry no device preference of this machine. A render that fails after the scene was built SHALL still leave the `.blend`.

#### Scenario: Open it elsewhere
- **WHEN** the `.blend` of the example is downloaded and opened on a Mac
- **THEN** it shows the same scene and plays the same animation as the clip

#### Scenario: A failed render
- **WHEN** the render of a built scene fails after the build
- **THEN** the `.blend` is there and the error says the render failed
