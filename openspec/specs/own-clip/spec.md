# own-clip Specification

## Purpose
TBD - created by archiving change add-own-clip. Update Purpose after archive.

## Requirements

### Requirement: A scene can use a clip that already exists

A scene of a video project SHALL accept `clip`, the name of a video file (`.mp4`, `.mov`, `.webm` or `.mkv`, a plain name, not a path) in the project's `sources/`. The scene SHALL then use that video instead of a generated picture and animation. The validation SHALL reject a `clip` that is not a valid name, does not exist in `sources/`, is combined with `image`, `init_image`, `animate`, `motion_prompt`, `animation` or `render`, or is set in a project that is not video, and SHALL warn that `prompt`, `negative_prompt`, `lora` and `variants` are ignored.

#### Scenario: A valid clip
- **WHEN** a scene has `clip: stand-up.mp4` and the file exists in `sources/`
- **THEN** the configuration is valid and no model is needed for that scene

#### Scenario: Missing file
- **WHEN** the file is not in `sources/`
- **THEN** the validation names the scene and the file

#### Scenario: Exclusive with the picture
- **WHEN** the scene also has `image` or `animate: true`
- **THEN** the validation rejects it

### Requirement: A scene with a clip is assembled from it

The `render` phase of such a scene SHALL write the first frame of the clip, fitted to the scene's resolution, as the scene picture, without loading any model; the `animate` phase SHALL NOT run. The `clip` phase SHALL build the scene's clip from the file in `sources/` with the scene's narration or silence, cover-cropped to the project's resolution and converted to its frame rate, cut to the scene's length when longer and slowed up to 2x and repeated when shorter. The scene's cache key SHALL depend on the content of the file, not its name.

#### Scenario: Longer clip
- **WHEN** the clip lasts 4 s and the scene 1.5 s
- **THEN** the scene's clip lasts 1.5 s and starts at the first frame of the file

#### Scenario: Replaced file
- **WHEN** the file in `sources/` is replaced by another with the same name
- **THEN** the scene is made again on the next run

#### Scenario: No GPU needed
- **WHEN** every scene has its own picture or its own clip
- **THEN** the run does not require `sd`

### Requirement: The clip of each scene is kept

After the clip phase of a video scene (made or found in the cache), the orchestrator SHALL keep its clip at `out/clips/<scene>.mp4`, whether or not `tmp/` is kept. The editor SHALL preview a scene with that file, or with `tmp/scene_<scene>.mp4` when the first does not exist, and SHALL say when neither exists.

#### Scenario: Run without keeping tmp
- **WHEN** a run finishes without `--keep-tmp`
- **THEN** `out/clips/` holds one clip per scene and the editor plays them

### Requirement: The Mi clip option in the editor

The scene form of a video project SHALL offer the scene's own clip next to its own picture: choosing a video from `sources/` or uploading one, with the preview of the chosen file.

#### Scenario: Choose a clip
- **WHEN** the user picks «Mi clip» and a video of `sources/`
- **THEN** the scene is saved with `clip` and without `prompt`-related fields

### Requirement: The dataset export notes own clips

Exporting a project as a dataset SHALL list in its notes each scene that uses its own clip, as it does for own pictures.

#### Scenario: Export
- **WHEN** a project with an own clip is exported as a dataset
- **THEN** the notes name the scene and say a dataset cannot carry the clip

### Requirement: A clip from another project is copied, and the user is told

The scene form SHALL offer the kept clips of the other projects (`out/clips/` of each) as a source of «Mi clip». Choosing one SHALL **copy** it into this project's `sources/` (never overwriting a file, taking a free name), so the scene depends only on its own project: deleting the other project, or deleting its runs with their files, SHALL NOT affect it. The form SHALL say so when a clip comes from another project. The confirmations that delete a project, or a run with its files, SHALL name the kept clips among what is removed.

#### Scenario: Use a clip of another project
- **WHEN** the user picks the kept clip of a scene of another project
- **THEN** it is copied into `sources/`, the scene uses the copy and the form says it is a copy

#### Scenario: The other project is deleted
- **WHEN** the project the clip came from is deleted afterwards
- **THEN** the scene still has its clip and the next run makes it as before

#### Scenario: The warning on delete
- **WHEN** the user confirms the deletion of a project or of a run's files
- **THEN** the message lists the kept clips with the other things removed

### Requirement: No clip is left loose

Clips SHALL NOT outlive what uses them. After each run the orchestrator SHALL remove from `out/clips/` the clips of scenes that are no longer in the project. When the project is saved, a video in `sources/` that the previously saved configuration used as a scene's clip and the new one no longer uses (the scene was removed, renamed or given another clip) SHALL be removed from `sources/`, unless another scene still uses it. The listing of `sources/` SHALL say which scenes use each video, so a video nobody uses (for example one uploaded and then abandoned) is visible and can be deleted from the sources list. Deleting a project, or a run with its files, SHALL remove the kept clips as the project-management capability says.

#### Scenario: A scene is removed
- **WHEN** a scene is deleted from the project and the run is made again
- **THEN** its clip is no longer in `out/clips/`

#### Scenario: The scene changes clip
- **WHEN** the user gives a scene another clip and saves
- **THEN** the video it used before is removed from `sources/`, unless another scene uses it

#### Scenario: Shared clip
- **WHEN** two scenes use the same video and one stops using it
- **THEN** the file stays in `sources/` for the other

#### Scenario: Abandoned upload
- **WHEN** a video was uploaded to `sources/` and no scene uses it
- **THEN** the sources list marks it as unused and offers to delete it

### Requirement: A scene can use only part of its own clip

A scene with `clip` or with `animate: true` SHALL accept `clip_start` and `clip_end`, in seconds, both optional (the start defaults to 0 and the end to the end of the video). For a scene with `clip` they count from the start of the file; for an animated scene they count from the start of the generated animation and are limited to its forward part (the reverse half of the ping-pong is not selectable). The validation SHALL reject a negative `clip_start`, a `clip_end` that is not greater than `clip_start`, and either field in a scene with neither `clip` nor `animate: true`. The scene SHALL use only that part of the video: the part is cut exactly and fitted to the scene's length as a whole video is (cut when longer; slowed up to 2x and repeated, the part and not the whole file, when shorter). For a scene with `clip` the first frame of the part is the scene picture. A start or an end beyond the length of the video (of its forward part for an animation) SHALL stop the run with an error that names the scene and that length. The trim SHALL be part of the scene's clip cache key and SHALL NOT make the animation be generated again.

#### Scenario: The middle of a clip
- **WHEN** a scene has `clip_start: 0.5`, `clip_end: 1.5` and a 1 s scene
- **THEN** the scene's clip is that second of the file and the scene picture is the frame at 0.5 s

#### Scenario: A part shorter than the scene
- **WHEN** the part lasts 1 s and the scene 3 s
- **THEN** the part is slowed by up to 2x and repeated to fill the scene

#### Scenario: Trimming a generated animation
- **WHEN** an animated scene has `clip_start: 0.4` and `clip_end: 1.6`
- **THEN** the scene's clip is that part of the forward animation, and the animate phase is not run again

#### Scenario: Changing the trim
- **WHEN** only `clip_start` changes
- **THEN** the scene is made again on the next run and the other scenes are not

#### Scenario: Only the trim changes
- **WHEN** the trim of an animated scene changes and nothing else
- **THEN** the animation stays cached and only the clip phase runs

#### Scenario: Past the forward part
- **WHEN** the end is later than the forward part of the animation
- **THEN** the run stops with an error naming the scene and the length of the part

#### Scenario: Invalid trim
- **WHEN** `clip_end` is not greater than `clip_start`, or the scene has neither `clip` nor `animate: true`
- **THEN** the validation names the scene and the field

### Requirement: The editor trims the clip with two numbers

The clip picker SHALL offer From and To fields in seconds, buttons that take the instant the preview is at, and the length of the chosen part. Leaving both empty SHALL use the whole clip.

#### Scenario: Mark from the preview
- **WHEN** the user pauses the preview at 0.8 s and presses the button of From
- **THEN** From becomes 0.8 and the scene is saved with `clip_start: 0.8`

### Requirement: The editor trims a generated animation too

The scene form SHALL show the From and To fields in a scene that animates, with the animation's preview when it exists and the length of the chosen part, and the fields SHALL work without the preview.

#### Scenario: Animated scene
- **WHEN** the user turns on animation for a scene
- **THEN** From and To are offered and are saved as `clip_start` and `clip_end`

### Requirement: A scene can set the speed of its video

A scene with `clip` or with `animate: true` SHALL accept `clip_speed`, a multiplier from 0.25 to 4 (1 is the video's own speed, 0.5 half speed, 2 double speed). The validation SHALL reject a value outside that range, and the field in a scene with neither `clip` nor `animate: true`. When set, the part of the video the scene uses (the whole video or its trimmed part) SHALL play at exactly that speed for as long as the scene lasts: cut when it lasts longer, repeated when it lasts less, and without the automatic slowdown that applies when the field is absent. When absent, the automatic behaviour SHALL NOT change. The speed SHALL be part of the scene's clip cache key and SHALL NOT make a generated animation be made again.

#### Scenario: Half speed
- **WHEN** a scene has `clip_speed: 0.5` and its part lasts 1 s
- **THEN** the part plays over 2 s and the video of a 2 s scene is that one slow pass

#### Scenario: Faster than the scene needs
- **WHEN** a scene has `clip_speed: 2`, a 2 s part and a 3 s scene
- **THEN** the part plays in 1 s and is repeated to fill the scene

#### Scenario: Absent
- **WHEN** a scene has no `clip_speed`
- **THEN** its clip is built exactly as before

#### Scenario: Out of range
- **WHEN** `clip_speed` is 0.1 or 5, or the scene has neither `clip` nor `animate: true`
- **THEN** the validation names the scene and the field

#### Scenario: Only the speed changes
- **WHEN** `clip_speed` of an animated scene changes and nothing else
- **THEN** the animation stays cached and only the clip phase runs

### Requirement: The editor sets the speed with a number

The From and To section SHALL have a Speed field, empty meaning automatic, saved as `clip_speed`.

#### Scenario: Set the speed
- **WHEN** the user writes 0.5 in Speed
- **THEN** the scene is saved with `clip_speed: 0.5`, and clearing the field removes it

### Requirement: The complete animation of a generated scene is kept too

After the animate phase of a scene (made or found in the cache), the orchestrator SHALL keep its complete animation at `out/clips/<scene>.anim.mp4`, whether or not `tmp/` is kept. After each run it SHALL remove from `out/clips/` any `.anim.mp4` of a scene that no longer exists or no longer animates, as it removes the clips of scenes that no longer exist. `GET /api/clips` SHALL say for each entry whether it is the finished clip or the complete animation, and copying a kept clip into another project SHALL accept which of the two. The clip picker SHALL offer both for a generated scene of another project, marking the animation as complete (forward and back), and the copy of an animation SHALL be named so that it cannot be confused with the clip of the same scene. Kept animations SHALL go with the kept clips when the run's files or the project are deleted.

#### Scenario: Run without keeping tmp
- **WHEN** a run with an animated scene finishes without `--keep-tmp`
- **THEN** `out/clips/` holds that scene's `.mp4` and its `.anim.mp4`

#### Scenario: Reuse the whole animation
- **WHEN** the user picks the complete animation of a scene of another project
- **THEN** it is copied into `sources/` as `<project>-<scene>-anim.mp4` and can be trimmed with From and To

#### Scenario: The scene stops animating
- **WHEN** a scene is changed to not animate and the run is made again
- **THEN** its `.anim.mp4` is no longer in `out/clips/`

#### Scenario: Delete the run's files
- **WHEN** a run is deleted with its files
- **THEN** the kept animations go with the kept clips

### Requirement: Each kept clip says who made it

Each clip of `GET /api/clips` SHALL carry `client` (the client of its project) and, for a clip of a managed project, `project_kind` (the clip's own `kind`, `clip` or `animation`, is unchanged). The request SHALL accept `managed=include` (the clips of managed projects are listed too; any other value is a `400`) and `client=a,b` (only clips of those clients); without them the list is what it was.

#### Scenario: A managed project's clip
- **WHEN** `GET /api/clips?managed=include` and a generation keeps a clip
- **THEN** the clip is listed with its `client` and `project_kind: "generation"`
