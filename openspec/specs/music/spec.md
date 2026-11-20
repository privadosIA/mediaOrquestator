# music Specification

## Purpose
Creates complete songs from a text prompt and optional lyrics, locally and offline, as an output independent of video: a lyrics-and-style composition step followed by an audio synthesis step, each in its own short-lived process.

## Requirements

### Requirement: A track describes one song request

A project in music mode SHALL declare a `tracks` list. Each track SHALL have an `id` (unique, filename-safe) and a `prompt` (the style/caption description). A track MAY have `lyrics` (multiline text, with section tags such as `[Verse]` and `[Chorus]`), `instrumental` (bool, default `false`), `duration_seconds`, `bpm`, `keyscale`, `timesignature`, `vocal_language`, `seed`, `variants` (int ≥ 1, default 1) and `use_lm` (bool). Fields left unset SHALL fall back to `defaults.music` and then to the engine's own defaults, except `vocal_language`, whose fallback is defined by the requirement "The vocal language is deduced from the lyrics when none is set"; the orchestrator SHALL NOT send fields the engine does not define.

#### Scenario: Minimal track
- **WHEN** a track has only `id` and `prompt`
- **THEN** the project is valid and the song is composed from the prompt alone

#### Scenario: Track without prompt
- **WHEN** a track has no `prompt`
- **THEN** validation fails naming the track and the field, before any process runs

### Requirement: use_lm defaults from the presence of lyrics

When a track does not set `use_lm`, it SHALL default to `false` if the track has non-empty `lyrics` and to `true` otherwise. With `use_lm` false the `compose` phase SHALL be skipped for that track and the caption and lyrics SHALL reach the synthesis step verbatim. With `use_lm` true the `compose` phase SHALL run first and its result (final lyrics, audio codes and resolved metadata) SHALL feed `synth`. An instrumental track SHALL NOT sing lyrics.

#### Scenario: Own lyrics are sung as written
- **WHEN** a track has lyrics and does not set `use_lm`
- **THEN** no `compose` phase runs for it
- **AND** the lyrics used for synthesis equal the track's lyrics

#### Scenario: Caption only
- **WHEN** a track has no lyrics, is not instrumental and does not set `use_lm`
- **THEN** the `compose` phase runs and produces the lyrics that are sung

#### Scenario: Explicit override
- **WHEN** a track has lyrics and sets `use_lm: true`
- **THEN** the `compose` phase runs and the manifest records both the lyrics requested and the lyrics sung

### Requirement: The vocal language is deduced from the lyrics when none is set

For a track that composes a new song (no `task`) and is not instrumental, the request SHALL always carry a `vocal_language`. The track's own value wins, then `defaults.music.vocal_language`. When neither is set, the language SHALL be deduced from the track's `lyrics` by a local detector: text in Hangul → `ko`, Japanese kana → `ja`, Han ideographs without kana → `zh`, Cyrillic → `ru`; for Latin script, the language among `es`, `en`, `fr`, `de`, `it`, `pt` whose frequent words and distinctive characters best match, provided the match is clear. With no lyrics, or no clear match, the language SHALL be `en`. The deduction SHALL be deterministic (the same lyrics always give the same language), SHALL NOT use the network, and SHALL NOT change the track's stored `vocal_language`. An instrumental track and a track with a `task` SHALL be unchanged.

#### Scenario: Spanish lyrics
- **WHEN** a non-instrumental track has Spanish lyrics and no `vocal_language`
- **THEN** the request carries `vocal_language` `es`

#### Scenario: Other scripts
- **WHEN** the lyrics are in Korean, Japanese, Chinese or Russian script
- **THEN** the request carries `ko`, `ja`, `zh` or `ru` respectively

#### Scenario: No lyrics
- **WHEN** a non-instrumental track has a prompt in Spanish and no lyrics
- **THEN** the request carries `vocal_language` `en`

#### Scenario: Unclear text
- **WHEN** the lyrics are too short or too mixed for a clear match
- **THEN** the request carries `en`

#### Scenario: Explicit language wins
- **WHEN** the track sets `vocal_language: fr`, or `defaults.music.vocal_language` is `fr` and the track sets none, and the lyrics are Spanish
- **THEN** the request carries `fr`

#### Scenario: Instrumental and task tracks
- **WHEN** a track is instrumental, or has a `task`
- **THEN** its request carries the same language fields as before this requirement

#### Scenario: The deduced language changes the cache key
- **WHEN** a track without language is run, and then its lyrics change from Spanish to English
- **THEN** the song is composed again

### Requirement: Each phase is a short-lived process

The `compose` phase SHALL run the engine's language-model program and the `synth` phase SHALL run its synthesis program, each as its own subprocess per track variant, through the same command-runner abstraction as the other phases. Flags SHALL be centralized in one place. The orchestrator SHALL NOT use a resident server process for the engine. No model SHALL stay loaded after a phase ends.

#### Scenario: Process ends after the phase
- **WHEN** `compose` or `synth` finishes, successfully or not
- **THEN** its process has exited and holds no GPU memory

#### Scenario: Engine failure
- **WHEN** an engine process exits non-zero
- **THEN** the phase emits an `error` event with the engine's message and the run stops with a non-zero exit

### Requirement: Variants and seeds follow the images rule

For a track with `variants: N`, the variant `i` (0-based) SHALL use seed `seed + i` when the track sets an explicit seed. Without an explicit seed each variant SHALL get a random 32-bit seed drawn by the orchestrator and recorded in the manifest. Variants SHALL run one after the other for a given track.

#### Scenario: Explicit seed
- **WHEN** a track has `seed: 100` and `variants: 3`
- **THEN** the variants use seeds 100, 101 and 102

#### Scenario: Random seed is recorded
- **WHEN** a track has no seed
- **THEN** the effective seed of every variant appears in `manifest.json`

### Requirement: Output files and layout

The `export` phase SHALL write the final audio to `out/<project>/<id>.<ext>` for a single variant and `out/<project>/<id>_v<N>.<ext>` (`N` 1-based) for several, where `<ext>` is the configured `output_format` (`mp3` by default, or `wav`). Output path collisions SHALL be rejected during validation. The final audio SHALL be stereo at the engine's native sample rate.

#### Scenario: Single variant
- **WHEN** track `intro` has one variant and `output_format` is `mp3`
- **THEN** `out/<project>/intro.mp3` exists after the run

#### Scenario: Several variants
- **WHEN** track `intro` has `variants: 2`
- **THEN** `out/<project>/intro_v1.mp3` and `out/<project>/intro_v2.mp3` exist

### Requirement: The manifest makes every song reproducible

For each exported variant the music `manifest.json` SHALL record: track id, variant index, effective seed, final caption, lyrics requested, lyrics sung, resolved bpm, duration, key and time signature, `use_lm`, the model files used, every generation parameter sent to the engine, and the file path. It SHALL be written atomically.

#### Scenario: Reproduce from the manifest
- **WHEN** a manifest entry's recorded parameters and seed are used to run the engine again with the same models
- **THEN** the same request is produced

#### Scenario: Lyrics differ after composition
- **WHEN** the language model changes the lyrics of a track that set `use_lm: true`
- **THEN** the manifest shows the requested lyrics and the sung lyrics as separate fields

### Requirement: Models come from a configurable directory

The engine models SHALL be read from `defaults.music.models_dir` (default `models/music`). The language model, the synthesis model, the text encoder and the VAE files SHALL be resolved there. A missing model file SHALL fail validation with a message naming the file and pointing to the Models screen or catalog, before any process runs.

#### Scenario: Missing DiT file
- **WHEN** the configured synthesis model is not in the models directory
- **THEN** the run fails before `compose` starts, naming the file

### Requirement: The caption is the prompt preceded by the style prefix

The caption sent to the engine for a track SHALL be its `prompt` when no style prefix is set, and `<style_prefix>, <prompt>` when one is. The prefix SHALL apply to every track, instrumental ones included, and SHALL NOT be added to the lyrics. Because the composed caption is an input of the compose and synth requests, changing the prefix SHALL invalidate the cache of the tracks it affects, and the manifest SHALL record the caption actually used.

#### Scenario: Prefix is prepended
- **WHEN** the prefix is `warm analog` and a track's prompt is `piano ballad`
- **THEN** the caption sent to the engine is `warm analog, piano ballad`

#### Scenario: Lyrics are untouched
- **WHEN** a track has lyrics and a prefix is set
- **THEN** the lyrics sent to the engine are exactly the track's lyrics

#### Scenario: Changing the prefix reruns the tracks
- **WHEN** a cached project's prefix changes
- **THEN** the next run composes and synthesizes its tracks again

### Requirement: The synthesis request carries the adapter

When a track resolves to an adapter, the synthesis request SHALL name it and its scale, and the synthesis process SHALL be given the adapters directory. The language-model phase SHALL NOT depend on the adapter. The synthesis memory reservation SHALL include the adapter's files. The manifest SHALL record the adapter and its scale among the generation parameters.

#### Scenario: Request names the adapter
- **WHEN** a track resolves to adapter `chinese-new-year` with scale 0.8
- **THEN** the synthesis request has `adapter` and `adapter_scale` 0.8 and the process receives the adapters directory

#### Scenario: No adapter, no change
- **WHEN** no adapter applies
- **THEN** the request and the process arguments are exactly as before

#### Scenario: Manifest records it
- **WHEN** a song was made with an adapter
- **THEN** its manifest entry shows the adapter and scale among its parameters

### Requirement: Task tracks synthesize from a source without composing

For a track with a task, the pipeline SHALL NOT run the language model. The synthesis request SHALL carry `task_type` and the task fields (`audio_cover_strength`, `repainting_start`, `repainting_end`, `track`), SHALL NOT carry a duration, and the synthesis program SHALL be given `--src-audio` with the project-relative path of the source; for `cover-nofsq` it SHALL also be given `--ref-audio` with the same file. Variants, seed, adapter, output format and export SHALL work as for a new song.

#### Scenario: Cover
- **WHEN** a track with `task: cover` and `cover_strength: 0.6` runs
- **THEN** no `compose` phase runs and the program receives `--src-audio sources/<file>` and a request with `task_type: cover` and `audio_cover_strength: 0.6`

#### Scenario: Close cover
- **WHEN** a track has `task: cover-nofsq`
- **THEN** the program receives `--src-audio` and `--ref-audio` with the same file

#### Scenario: Repaint region
- **WHEN** a repaint track has `repaint_start: 10` and `repaint_end: 25`
- **THEN** the request has `repainting_start: 10` and `repainting_end: 25`

#### Scenario: Extract
- **WHEN** an `extract` track has `stem: vocals`
- **THEN** the request has `task_type: extract` and `track: vocals`

#### Scenario: New songs are unchanged
- **WHEN** a track has no task
- **THEN** its request and arguments are exactly as before

### Requirement: The server engine runs the same requests

With `engine: server`, the run SHALL start `ace-server` (loopback only, free port, `--models`, `--adapters` when present, `--keep-loaded` unless `keep_loaded` is false) once per GPU in use, wait until it answers `/health`, send compose and synth jobs to it with the same requests the programs would receive (source audio and, for a close cover, the reference as multipart parts), and stop it when the run ends, fails or is cancelled. Exported audio, manifest, seeds, cache hashes and events SHALL be the same as with `cli`.

#### Scenario: Compose and synth through the server
- **WHEN** a track runs with `engine: server`
- **THEN** a compose job and a synth job are submitted and the song is exported as with the programs

#### Scenario: One server per run
- **WHEN** several tracks run on one GPU
- **THEN** a single server is started and it is stopped at the end

#### Scenario: Cancelled run
- **WHEN** the run is cancelled while a job is running
- **THEN** the job is cancelled and the server process ends

#### Scenario: Server fails to start
- **WHEN** the server exits before it is ready
- **THEN** the run fails with the server's output

#### Scenario: Switching engines keeps the cache
- **WHEN** a project that ran with `cli` runs with `server` and nothing else changed
- **THEN** every phase is a cache hit

#### Scenario: Cover through the server
- **WHEN** a track with a task runs with `engine: server`
- **THEN** the source audio is sent with the job

### Requirement: The reference reaches the engine as timbre reference

For a track with a `reference`, the synthesis SHALL be given the file as reference audio (`--ref-audio` for the program, the `ref_audio` part for the server) and no source audio; the language model phase SHALL be unchanged.

#### Scenario: Program arguments
- **WHEN** a track with `reference: sound.mp3` is synthesized
- **THEN** the program receives `--ref-audio sources/sound.mp3` and no `--src-audio`

#### Scenario: Server
- **WHEN** the same track runs with the server engine
- **THEN** the job carries the file as the `ref_audio` part and no `audio` part

#### Scenario: Compose untouched
- **WHEN** only the reference of a cached track changes
- **THEN** the compose phase is a hit and the synth phase runs

### Requirement: A music project may declare mixes of clips

A music project MAY declare `mixes`, a list whose entries each have an `id` (unique among mixes and distinct from every track `id`; letters, digits, `.`, `_` or `-`) and a non-empty `clips` list. A clip SHALL name its audio with exactly one of `track` (the `id` of a track of the project, with an optional `variant`, 1-based, default 1) or `file` (a name in the project's sources folder). A clip MAY set `start` (seconds from the start of the mix, default 0, ≥ 0), `gain` (0 to 2, default 1), `mute` (bool, default false), `fade_in` and `fade_out` (seconds ≥ 0, default 0) and `trim_start` / `trim_end` (seconds of the original to use, ≥ 0; `trim_end` greater than `trim_start`; default the whole file). `mixes` SHALL be accepted only in a music project. Validation SHALL name the mix and clip and the field for: a duplicate or invalid id, an id that equals a track id, no clips, a clip with both or neither of `track` and `file`, an unknown track or variant, a `file` that is not a valid source name, and any value out of range. A fade longer than the audible length of its clip SHALL only warn.

#### Scenario: Layering a voice over a base
- **WHEN** a mix has clip A (`track: base`, `start: 0`) and clip B (`track: voz`, `start: 10`, `gain: 0.8`)
- **THEN** the project is valid and B enters at 10 s over A

#### Scenario: Clip with two sources
- **WHEN** a clip sets both `track` and `file`
- **THEN** validation fails naming the mix, the clip and both fields, before any process runs

#### Scenario: Id clash with a track
- **WHEN** a mix has the same `id` as a track
- **THEN** validation fails naming both

#### Scenario: Unknown track
- **WHEN** a clip names a track or variant that does not exist
- **THEN** validation fails naming the clip and the missing track or variant

### Requirement: A mix phase renders each mix with ffmpeg

After the tracks, the pipeline SHALL run a `mix` phase per mix (events `phase_start`/`phase_end` with the mix id) that renders `out/<project>/<mix id>.<format>`, the format being the project's music output format. Each non-muted clip SHALL be cut to `trim_start`–`trim_end`, given its fades (in at the start of the used part, out at its end), scaled by `gain`, delayed to `start` and converted to a common rate and channel layout; the clips SHALL be summed without automatic normalisation and the result limited so it never clips. The mix lasts until its last clip ends. A muted clip is not rendered and does not affect the duration. A clip whose audio file does not exist (a track not yet generated, a missing source) SHALL fail the mix with a message naming the clip and how to produce the file. Without `mixes`, no `mix` phase runs and nothing changes.

#### Scenario: Layers and sequence
- **WHEN** a mix has a 20 s clip at 0 s and a 10 s clip at 15 s
- **THEN** the file lasts 25 s and the two clips sound together between 15 s and 20 s

#### Scenario: Gain and fades
- **WHEN** a clip has `gain: 0.5` and `fade_in: 2`
- **THEN** it enters at half level over its first 2 s

#### Scenario: Muted clip
- **WHEN** a clip is muted
- **THEN** it is absent from the sound and from the duration

#### Scenario: Missing audio
- **WHEN** a clip names a track that has no exported file yet
- **THEN** the mix fails naming the clip and says to generate the track first, and other mixes still run

#### Scenario: No clipping
- **WHEN** two loud clips overlap
- **THEN** the output peak stays below full scale

### Requirement: Mixes are cached by content and selected with --only

A mix SHALL be re-rendered only when its description or the content of any of its input files changes (or with `--force`); an unchanged mix SHALL be reported as skipped. `--only <id>` SHALL process only the mix or track with that id; a run restricted to a track SHALL NOT render mixes, and a run restricted to a mix SHALL NOT generate tracks.

#### Scenario: Unchanged mix is skipped
- **WHEN** a mix is run twice with no change
- **THEN** the second run skips the `mix` phase

#### Scenario: A track changed
- **WHEN** the audio of a track used by a mix is regenerated
- **THEN** the mix is rendered again

#### Scenario: Only a mix
- **WHEN** the run is restricted to a mix id
- **THEN** no track is generated and only that mix is rendered

### Requirement: The caption ends with the chosen voice

When a track has a voice (its own `vocals`, else `defaults.music.vocals`), the caption sent to the engine SHALL end with the English phrase of that voice, after the style prefix and the prompt. Instrumental tracks and `extract` tracks SHALL NOT get it, and a track without a voice SHALL keep exactly the caption it had before. Because the caption is an input of the requests, changing the voice SHALL invalidate the cache of the track.

#### Scenario: Voice appended
- **WHEN** the prefix is `warm`, the prompt `piano ballad` and the voice `female`
- **THEN** the caption is `warm, piano ballad, female vocals`

#### Scenario: Track overrides the default
- **WHEN** the project default is `male` and the track sets `female`
- **THEN** the caption ends with `female vocals`

#### Scenario: Instrumental has no voice
- **WHEN** an instrumental track sets a voice
- **THEN** the caption is unchanged

### Requirement: The manifest records what mixes were made from

Each song entry of the music manifest SHALL carry `sha256`, the content hash of its exported file. Each mix entry SHALL carry `inputs`, the project-relative file and content hash of every unmuted clip it read, as they were when the mix was rendered, including when the mix was kept because nothing changed.

#### Scenario: Hashes recorded
- **WHEN** a project with a mix is created
- **THEN** every song entry has a `sha256` and the mix's `inputs` list those songs with the same hashes

#### Scenario: A regenerated song differs
- **WHEN** a song is created again with another seed and the mix is not rendered
- **THEN** the song's `sha256` differs from the one in the mix's `inputs`

### Requirement: Rendering applies the master settings

A mix SHALL be rendered by summing its clips, then applying its master volume when it is not 1, then, when `normalize` is set, normalising its loudness to -14 LUFS, and finally the limiter. The master settings SHALL be part of the mix's hash only when they are not at their defaults, SHALL be recorded in the mix's manifest entry (`gain` when not 1, `normalize` when set), and changing either SHALL render the mix again.

#### Scenario: Master volume
- **WHEN** a mix has `gain: 0.5`
- **THEN** its peak is about 6 dB lower than without it

#### Scenario: Normalised loudness
- **WHEN** a quiet mix is normalised
- **THEN** its integrated loudness is close to -14 LUFS and its length is unchanged

#### Scenario: Existing mixes stay
- **WHEN** a mix without master settings is run after upgrading
- **THEN** it is skipped as unchanged

#### Scenario: Changing them re-renders
- **WHEN** the mix's gain or normalisation changes
- **THEN** only that mix is rendered again

### Requirement: Equal results are reused across cache keys

When a variant's compose or synth hash is not cached under its own key but another cached entry of the project has the same hash and its file is still on disk, the run SHALL copy that result for the variant and treat the phase as a hit, unless `--force` is set.

#### Scenario: Variant kept from several
- **WHEN** a track with two variants and seed 100 is created, then changed to one variant with seed 101
- **THEN** the engine does not run again and the track's song is the one made as the second variant

### Requirement: Superseded songs are removed

After a run, the songs the previous manifest listed for a track the run processed that the new manifest does not include SHALL be deleted. Songs of tracks the run did not process, of tracks no longer in the project, and any file listed by the new manifest SHALL be left alone.

#### Scenario: From three variants to one
- **WHEN** a track goes from two variants to one and is created again
- **THEN** `a_v1.mp3` and `a_v2.mp3` are removed and `a.mp3` exists

#### Scenario: Only one track
- **WHEN** the project's tracks changed but the run is limited to one of them
- **THEN** the other tracks' songs stay

### Requirement: A rendered mix records its loudness

After rendering a mix the run SHALL measure its integrated loudness and record it in the mix's manifest entry as `lufs`, rounded to a tenth. A mix whose loudness cannot be measured SHALL go without it, and a mix skipped as unchanged SHALL keep its entry as it was.

#### Scenario: Recorded
- **WHEN** a mix is rendered
- **THEN** its manifest entry has `lufs`

#### Scenario: Unchanged mix
- **WHEN** a mix is skipped as unchanged
- **THEN** ffmpeg is not run again for it

### Requirement: Exported mp3 files carry tags

Every mp3 the export phase writes SHALL carry an ID3v2.3 tag, written without changing the audio bytes and replacing a tag the file already had, with the title (the track's `title`, or its id with spaces for `_` and `-`; "(vN)" is added to a song with several versions), the artist (`defaults.music.artist`, or the project's name), the album (the project's name), the track number, the final caption as comment and the lyrics sung, as UTF-16 text. A wav SHALL be left as it was made. If the tag cannot be written the song SHALL stay as exported and a warning SHALL say so.

#### Scenario: Tagged song
- **WHEN** a track `night_drive` with lyrics is exported as an mp3
- **THEN** its tag has the title "night drive", the project's name as artist and album, track 1, the caption and the lyrics, and the audio is unchanged

#### Scenario: Versions
- **WHEN** a track has two versions
- **THEN** the second is titled "… (v2)"

### Requirement: Songs can have a cover

With `defaults.music.cover: auto` every track whose own `cover` is not `off` SHALL get a cover: a `cover` phase after synth SHALL make one picture per track, however many versions it has, with the project's image model (`defaults.model` or `defaults.model_set`) from the track's caption (with the style prefix), at the project's resolution (768x768 when unset), sampler (euler) and steps (20 when unset), with a seed that is the track's or made from its id; its result SHALL be cached by the same hash as an image and found in the cache by later runs. The picture SHALL be embedded in the mp3 as the front cover and copied to `out/<project>/<id>.png`, and the manifest entry SHALL record `title` and `cover`. A track's `cover` SHALL be `off` or a picture of `sources/` (png, jpg, jpeg or webp) that is used as it is, and wins over the project's. `auto` without an image model SHALL be refused by validation. When the picture cannot be made the song SHALL still be created, without a cover, and a warning SHALL say why.

#### Scenario: Automatic cover
- **WHEN** a project has `cover: auto` and an image model and a song is created
- **THEN** a picture is made, embedded and exported next to the song, and the manifest says where

#### Scenario: Cached
- **WHEN** the same project runs again
- **THEN** the cover phase is skipped

#### Scenario: Own picture or off
- **WHEN** a track names a picture of its sources, or `off`
- **THEN** no picture is made; the first is embedded as it is and the second has none

#### Scenario: The picture cannot be made
- **WHEN** the image program fails
- **THEN** the song is exported without a cover and a warning says the cover was skipped

### Requirement: Every exported song is checked

After exporting a song the pipeline SHALL measure its peak, its silences and its integrated loudness with ffmpeg and record them in the manifest entry as `check` (`peak_db`, `lufs`, `issues`). It SHALL report as issues, each with a warning event: `clipping` when the peak is above -0.3 dBFS, `silence` for each stretch under -50 dB of 4 seconds or more, `quiet` under -24 LUFS, `loud` above -8 LUFS, and `duration` when the real length differs from the requested one by more than 15 %. A song whose file and `sha256` equal those of the previous manifest entry SHALL keep that entry's check without being measured again. When ffmpeg is missing or measures nothing the entry SHALL have no `check` and the run SHALL NOT fail.

#### Scenario: A clipping song
- **WHEN** a song with a peak of -0.1 dBFS is exported
- **THEN** its `check.issues` contains `clipping` and the run emits one warning for it

#### Scenario: An unchanged song
- **WHEN** a run exports a song identical to the one in the previous manifest
- **THEN** ffmpeg is not run for it and its check is kept

### Requirement: Songs can be evened to a target loudness

When `defaults.music.loudness_lufs` is set (valid between -30 and -8) the export of each song SHALL measure its integrated loudness, apply the gain that reaches the target (at most 12 dB either way) followed by the export peak limiter, and encode it again at the configured mp3 bitrate; a song within 0.3 dB of the target, or silent, SHALL be left as it is. The manifest entry SHALL record `loudness_from` and `loudness_gain` when a gain was applied. The cached synthesis SHALL NOT be changed, so changing the target never regenerates a song. When the setting is absent songs SHALL be exported as the engine made them.

#### Scenario: A quiet song is raised
- **WHEN** a song measured at -20 LUFS is exported with a target of -14
- **THEN** the exported file is 6 dB louder and the entry records `loudness_from` -20 and `loudness_gain` 6

#### Scenario: Out of range
- **WHEN** the target is -5
- **THEN** the project is invalid

### Requirement: A project can make its songs with the HeartMuLa engine

`defaults.music.generator` SHALL be `ace-step` (also when absent) or `heartmula`. With `heartmula` the synthesis of a track SHALL send the lyrics as written and the tags of the track to the engine with a maximum length (the requested duration plus a margin, or 240 seconds when none is asked), the track's seed and, for the variants, one request per variant with its own seed; the song SHALL come back as a WAV and go through what every song goes through (ffmpeg to mp3, the limiter and the loudness target, the tags and cover of the file, the mixes and the videoclip). The `lm` phase of ACE-Step SHALL not run for these tracks. A track with no tags SHALL fail the run naming it and what is missing; the studio offers to write the tags from the track's style text with the assistant, and the user can edit them. A project that asks for `heartmula` when the engine is not available SHALL fail validation naming the engine and what is missing.

#### Scenario: A song
- **WHEN** a track with lyrics and tags runs with `generator: heartmula`
- **THEN** one request is sent per variant, the WAV becomes the track's mp3 with the usual loudness and tags, and no ACE-Step phase runs

#### Scenario: No tags
- **WHEN** a track of a HeartMuLa project has no tags
- **THEN** the run fails naming the track and saying that tags are needed, before any song is made

#### Scenario: Not available
- **WHEN** the engine is `heartmula` and neither a local nor an external engine is available
- **THEN** validation fails with the reason

### Requirement: The cache of a HeartMuLa song depends on what makes it

The cache key of a song made with HeartMuLa SHALL include the engine, the lyrics, the tags, the seed, the maximum length and the sampling parameters (temperature, top-k and guidance scale), and SHALL not include the models of ACE-Step; changing any of them SHALL make that song again and nothing else. A project that never used `heartmula` SHALL keep every key it had.

#### Scenario: Tags changed
- **WHEN** the tags of one track change
- **THEN** only that track's songs are made again

#### Scenario: Existing projects
- **WHEN** a project made with ACE-Step runs after this change
- **THEN** every cache key is the one it had

### Requirement: What HeartMuLa cannot do is not offered or is reported

With the `heartmula` engine, tracks that are tasks (remix, cover, lengthen) or that take a reference audio, and the fields `bpm`, `keyscale` and `timesignature`, SHALL be reported by validation as not supported by this engine (a warning for the fields, which are ignored, and an error for the tasks and the reference). The engine SHALL not be selectable for a project that has such tracks without saying so.

#### Scenario: A remix track
- **WHEN** a project with a remix track asks for `heartmula`
- **THEN** validation reports that tasks are not supported by this engine

#### Scenario: A bpm
- **WHEN** a track sets a bpm
- **THEN** a warning says that the engine ignores it

### Requirement: A project can make its songs with the DiffRhythm2 engine

`defaults.music.generator` SHALL accept `diffrhythm2` besides `ace-step` and `heartmula`. With it the synthesis of a track SHALL send the lyrics as written and the tags of the track (the style) to the engine with a maximum length (the requested duration plus a margin, never more than 210 seconds, and 210 when none is asked) and the track's seed (one request per variant, each with its own seed); the song SHALL come back as a WAV and go through what every song goes through (mp3, limiter, loudness target, tags, cover, mixes and the videoclip). The length asked for is the longest the song may take: the model stops by itself at the end of the lyrics. The `lm` phase of ACE-Step SHALL not run. A track with no tags SHALL fail the run naming it and what is missing.

#### Scenario: A song
- **WHEN** a track with English lyrics and tags runs with `generator: diffrhythm2`
- **THEN** one request is sent per variant and the WAV becomes the track's mp3 with the usual loudness and tags

#### Scenario: No tags
- **WHEN** a track of a DiffRhythm2 project has no tags
- **THEN** the run fails naming the track and saying that tags are needed, before any song is made

#### Scenario: Not available
- **WHEN** the engine is `diffrhythm2` and neither a local nor an external engine is available
- **THEN** validation fails with the reason

### Requirement: The cache of a DiffRhythm2 song depends on what makes it

The cache key of a song made with DiffRhythm2 SHALL include the engine, the lyrics, the tags, the seed, the maximum length and the guidance and the number of steps, and SHALL not include the models of ACE-Step or the tags or sampling parameters of HeartMuLa; changing any of them SHALL make that song again and nothing else. A project that never used `diffrhythm2` SHALL keep every key it had.

#### Scenario: Tags changed
- **WHEN** the tags of one track change
- **THEN** only that track's songs are made again

#### Scenario: Existing projects
- **WHEN** a project made with ACE-Step or HeartMuLa runs after this change
- **THEN** every cache key is the one it had

### Requirement: What DiffRhythm2 cannot do is refused or reported

With the `diffrhythm2` engine, validation SHALL refuse (an error naming the track) a track whose `vocal_language` is not `en` or is empty with lyrics in another language, an instrumental track, a task (remix, cover, repaint, extract, lengthen) and a reference audio; and SHALL warn (the field is ignored) about `bpm`, `keyscale`, `timesignature`, the vocal style and the ACE-Step sampling fields. An empty `vocal_language` SHALL be taken as English.

#### Scenario: Spanish lyrics
- **WHEN** a track of a DiffRhythm2 project has `vocal_language: es`
- **THEN** validation fails naming the track and saying that this engine reads English lyrics only

#### Scenario: An instrumental
- **WHEN** a track is instrumental
- **THEN** validation fails saying that this engine cannot make instrumentals

#### Scenario: A remix
- **WHEN** a project with a remix track asks for `diffrhythm2`
- **THEN** validation reports that tasks are not supported by this engine

#### Scenario: A bpm
- **WHEN** a track sets a bpm
- **THEN** a warning says that the engine ignores it
