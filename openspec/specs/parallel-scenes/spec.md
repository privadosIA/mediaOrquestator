# parallel-scenes Specification

## Purpose
Runs several scenes of one project at the same time, one per GPU, while keeping the machine's RAM within a budget so concurrent `sd` processes never exhaust memory.

## Requirements

### Requirement: Scenes are distributed across the listed GPUs

When more than one GPU is listed, the pipeline SHALL run one scene at a time on each listed GPU, handing the next unprocessed scene (in project order) to whichever GPU becomes free first. Every phase of a scene (narration, render, animate, clip, export) SHALL run for that scene on the GPU it was assigned, with that GPU as the only visible device for `sd`. With zero or one GPU listed the run SHALL behave exactly as before: scenes processed one after another, same invocations, same event order.

#### Scenario: two GPUs process two scenes at once
- **WHEN** a project with 4 scenes runs with GPUs `0,1`
- **THEN** scenes 1 and 2 start on GPUs 0 and 1 respectively before either finishes
- **AND** scene 3 starts on whichever GPU finishes its scene first
- **AND** every `sd` invocation of a scene carries the `CUDA_VISIBLE_DEVICES` of its assigned GPU

#### Scenario: single GPU is unchanged
- **WHEN** a run lists one GPU or none
- **THEN** the sequence of subprocess invocations and events equals the current sequential run

### Requirement: Output order does not depend on completion order

The final video's clips, the export manifest's entries, the exported file names and the cache manifest SHALL follow the order of the scenes in the project regardless of which scene finishes first. Random seeds for scenes without a seed SHALL be drawn in project order before any scene starts, so the same random source yields the same seeds with or without parallelism.

#### Scenario: a later scene finishes first
- **WHEN** scene 2 finishes before scene 1
- **THEN** the final video places scene 1's clip before scene 2's
- **AND** the export manifest lists scene 1 before scene 2

### Requirement: Concurrent sd processes share a RAM budget

Before launching an `sd` process the pipeline SHALL reserve an estimate of its memory: the total size of the model files it loads multiplied by 1.15. The budget SHALL be the memory available when the run starts minus 4 GiB. An invocation whose estimate does not fit alongside the reservations in force SHALL wait until enough is released. An invocation whose estimate exceeds the whole budget SHALL run alone once all other reservations are released. Reservations SHALL be released when the process ends, whether it succeeds or fails. When the available memory cannot be read, `sd` invocations SHALL be serialized. The wait SHALL count towards no phase duration: a phase starts when its memory is reserved.

#### Scenario: two small invocations overlap
- **WHEN** two animate invocations each estimate 9 GiB and the budget is 23 GiB
- **THEN** both run at the same time

#### Scenario: two large invocations serialize
- **WHEN** two render invocations each estimate 18 GiB and the budget is 23 GiB
- **THEN** the second waits until the first ends

#### Scenario: an oversized invocation still runs
- **WHEN** one invocation estimates more than the whole budget
- **THEN** it waits for all other reservations to be released and then runs alone
- **AND** the run does not deadlock

#### Scenario: memory cannot be read
- **WHEN** the available memory is unreadable
- **THEN** `sd` invocations run one at a time

### Requirement: A failing scene stops the whole run

When a scene fails, the pipeline SHALL cancel the scenes still running, SHALL NOT start further scenes, SHALL wait for running subprocesses to end and SHALL finish with the failing scene's error. The failing scene emits its error event. A scene that was in the middle of a phase when it was cancelled SHALL emit an error event for that phase saying it was stopped and why, so that every `phase_start` stays paired; a scene that had not started a phase emits nothing. The run's returned error SHALL be the first failure.

#### Scenario: one scene fails while another runs
- **WHEN** a scene on GPU 0 fails while a scene runs on GPU 1
- **THEN** the scene on GPU 1 is cancelled
- **AND** the run ends with the failing scene's error and starts no new scenes
- **AND** the cancelled scene emits an error event beginning with "stopped:" closing its open phase

#### Scenario: a waiting scene is cancelled silently
- **WHEN** a scene is waiting for RAM or for a GPU when another scene fails
- **THEN** it emits no event, because it had not started a phase

### Requirement: Free VRAM is checked on every listed GPU

When GPUs are listed, the free-VRAM warning SHALL be evaluated for each of them, naming the GPU in the warning.

#### Scenario: one of two GPUs is low
- **WHEN** GPU 0 has less free VRAM than the threshold and GPU 1 has more
- **THEN** one warning is emitted, naming GPU 0

### Requirement: Music processes reserve memory in the shared budget

Before launching `ace-lm` or `ace-synth` the pipeline SHALL reserve an estimate of its memory: the total size of the model files that process loads multiplied by 1.15, using the same budget and release rules as `sd`. The language model and the synthesis model SHALL be reserved separately because they run in different processes. Music processes and `sd` processes SHALL draw from the same budget.

#### Scenario: Language model and synthesis do not add up
- **WHEN** a track runs `compose` and then `synth`
- **THEN** the reservation of `compose` is released before `synth` reserves its own

#### Scenario: Oversized process
- **WHEN** one music process estimates more than the whole budget
- **THEN** it runs alone once other reservations are released

### Requirement: Tracks use the listed GPUs

With `--gpu` listing several GPUs in music mode, tracks SHALL be distributed across them with at most one music process per GPU at a time. The output order and the manifest order SHALL follow the declaration order of the tracks, not completion order. A failing track SHALL stop the run like a failing scene does.

#### Scenario: Two GPUs, two tracks
- **WHEN** two tracks run with `--gpu 0,1`
- **THEN** they may run at the same time, each pinned to a different GPU

#### Scenario: Order of results
- **WHEN** the second track finishes first
- **THEN** the manifest still lists the first track first

### Requirement: CPU phases may overlap the GPU phases when asked
When the overlap option is on, the pipeline SHALL be allowed to run the narration and probe of upcoming scenes, and the clip of a finished scene, while a GPU phase of another scene runs, with at most the configured number of scenes of look-ahead. A GPU phase SHALL never run two at a time on the same GPU. When the option is off, which is the default, the run SHALL behave exactly as the requirement on single-GPU runs states.

#### Scenario: narration of the next scene during a render
- **WHEN** the option is on, one GPU is listed and scene 1 is rendering
- **THEN** the narration of scene 2 may run at that moment
- **AND** scene 2's render does not start until scene 1's GPU phases end

### Requirement: Overlapping does not change what is produced
Whatever the order phases finish in, the final video, the export manifest, the exported names and the seeds SHALL be the same as without the option.

#### Scenario: same result with and without the option
- **WHEN** the same project runs with and without the option
- **THEN** the order of clips in the final video and of entries in the manifest is identical

#### Scenario: a failure while phases overlap
- **WHEN** the narration of a look-ahead scene fails while another scene renders
- **THEN** the run is cancelled, no further phase starts, every open phase is closed with an error event and the run ends with the first failure

#### Scenario: the option is off
- **WHEN** the option is not given
- **THEN** the sequence of subprocess invocations and events equals the current sequential run
