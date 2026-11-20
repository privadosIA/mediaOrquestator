# run-statistics Specification

## Purpose
Defines the statistics computed from the run history (counts, success rate, durations, time per phase, cache reuse, per-project and per-GPU figures, slowest runs) and the sampling of GPU and machine resources while a run is active, with their read-out and peaks.

## Requirements

### Requirement: Statistics are aggregated from the run history

`GET /api/stats` SHALL aggregate, for the runs created in a period (`since`, `until`; by default the last 30 days) and optionally one `project`, from the stored runs and their events: the number of runs per state and the success rate; per day, the runs per state and the seconds run; run duration (`finished_at − started_at`) total, mean, median and 95th percentile over the succeeded runs; the mean wait in the queue (`started_at − created_at`); per phase, its executions (`phase_end` events), its reuses (`skipped` events), and the total, mean and 95th-percentile `duration_ms`; the cache figures (phases executed, reused, and the share reused); per project and per GPU, the runs, the seconds and the success rate; and the ten slowest runs. Runs without events or without timestamps SHALL count in the totals but SHALL NOT distort the duration figures. The result SHALL be computed from the history as it is, so runs that predate any sampling are included.

#### Scenario: success rate and counts
- **WHEN** the period has three succeeded runs and one failed run
- **THEN** the totals say four runs, three succeeded, one failed, and a success rate of 75 %

#### Scenario: time per phase
- **WHEN** runs contain `phase_end` events for render with durations 10 s and 20 s and one `skipped` event for render
- **THEN** the render phase reports two executions, one reuse, a mean of 15 s and a total of 30 s

#### Scenario: cache share
- **WHEN** 6 phases were executed and 4 were reused
- **THEN** the cache figures say 10 phases, 4 reused and a share of 40 %

#### Scenario: filtering by project and period
- **WHEN** `project` and `since` are given
- **THEN** only the runs of that project created since then are aggregated

#### Scenario: a run without events
- **WHEN** an interrupted run has no events
- **THEN** it counts in the totals by state and contributes nothing to the phase or duration figures

#### Scenario: an empty period
- **WHEN** no run matches
- **THEN** the answer has zero totals and empty series, not an error

#### Scenario: a bad range
- **WHEN** `since` is after `until` or a timestamp is malformed
- **THEN** the answer is `400` with the standard error body

### Requirement: Resources are sampled while a run is active

While at least one run is active, the server SHALL sample every 2 seconds, for each GPU the run occupies, the used and total VRAM and the GPU utilization, and the machine's used and total RAM, and SHALL store the samples with the run, taking one more sample when the run starts as a reference. A run that does not name GPUs SHALL occupy every GPU `nvidia-smi` reports. When `nvidia-smi` is not available only the RAM SHALL be sampled. Sampling SHALL stop when the run ends, SHALL NOT happen when no run is active, and SHALL stop for a run that reaches 20 000 stored samples, which SHALL be marked as truncated. A failed sample SHALL NOT affect the run.

#### Scenario: samples during a run
- **WHEN** a run on GPU 1 lasts ten seconds
- **THEN** about five samples with GPU 1's VRAM, utilization and the machine's RAM are stored for it

#### Scenario: no run, no sampling
- **WHEN** no run is queued or running
- **THEN** nothing is sampled and `nvidia-smi` is not called

#### Scenario: no nvidia-smi
- **WHEN** `nvidia-smi` is not installed
- **THEN** RAM samples are stored and the run behaves as usual

#### Scenario: a failing sample
- **WHEN** reading the GPU fails once
- **THEN** that sample is skipped and the run continues

### Requirement: A run's resources are readable, with peaks, and labelled for what they measure

`GET /api/runs/{id}/resources` SHALL answer the sampling interval, whether sampling was truncated, per GPU its total VRAM and its samples (`t_ms` since the run started, used MB, utilization), the RAM total and samples, and the peaks (maximum VRAM per GPU in MB and as a percentage of its total, maximum RAM in MB and as a percentage). The VRAM SHALL be documented as that of the whole GPU during the run, which includes any other process using it. A run without samples SHALL answer empty lists and no peaks, not an error; an unknown run SHALL answer `404`.

#### Scenario: peaks
- **WHEN** a run's VRAM samples on GPU 1 reach 14 200 MB of 16 311 MB
- **THEN** the peak is 14 200 MB and 87 %

#### Scenario: an old run
- **WHEN** a run recorded before sampling existed is requested
- **THEN** the answer has empty series and no peaks

#### Scenario: an unknown run
- **WHEN** the id does not exist
- **THEN** the answer is `404`

#### Scenario: per-GPU peaks reach the statistics
- **WHEN** runs with samples are aggregated per GPU
- **THEN** each GPU row carries the highest VRAM percentage reached in the period and the mean of the runs' peaks

### Requirement: The statistics can be filtered and grouped by the kind of project

`GET /api/stats` SHALL accept `mode` with one or more of `video`, `images` and `music` separated by commas and SHALL then count only the runs of projects of those kinds; any other value SHALL be `400`. The answer SHALL carry `by_mode`, one group per kind with its runs, seconds and success rate (the runs of a project that no longer exists under the empty name), and each run of the duration and slowest tables SHALL carry the `mode` of its project when it is known.

#### Scenario: only songs

- **WHEN** a client requests `/api/stats?mode=music`
- **THEN** only the runs of music projects are counted and `by_mode` has the single group `music`

#### Scenario: an unknown kind

- **WHEN** a client requests `/api/stats?mode=lava`
- **THEN** the answer is `400`

### Requirement: The statistics can be limited to an origin

`GET /api/stats` SHALL accept `origin`, `desktop` or `mobile` (comma separated), and keep only the runs of projects made there (mobile: the name starts with `mobile-`). Anything else SHALL be `400`. Without it every run is counted.

#### Scenario: Mobile

- **WHEN** `origin=mobile` is asked
- **THEN** only the runs of `mobile-…` projects are counted

#### Scenario: Unknown

- **WHEN** `origin=tablet` is asked
- **THEN** the answer is `400`

### Requirement: The statistics can be limited to a server

`GET /api/stats` SHALL accept `server`, one or more server names separated by commas, and keep only the runs made by those servers (the history can be shared by several). Without it every run is counted. The Statistics page SHALL offer the choice only when the history has runs of more than one server, and the Runs history SHALL show the server of each run.

#### Scenario: One server

- **WHEN** `server=portatil` is asked
- **THEN** only the runs made by `portatil` are counted

#### Scenario: A single server

- **WHEN** the history has runs of one server only
- **THEN** the page does not offer the choice

### Requirement: The history gives the typical duration of a phase for a model, resolution and GPU
The server SHALL offer, from the stored runs, the median and 95th-percentile duration of a phase for a given model, resolution and GPU, with the number of executions it is based on. Runs that failed, were cancelled or were served from the cache SHALL NOT count. When fewer than the minimum number of executions exist, the server SHALL say there is not enough history instead of returning a figure.

#### Scenario: enough history
- **WHEN** five succeeded executions of the render phase exist for a model, resolution and GPU
- **THEN** the median, the 95th percentile and the count of five are returned

#### Scenario: not enough history
- **WHEN** fewer executions than the minimum exist
- **THEN** the answer says there is not enough history and gives no duration

#### Scenario: failed and reused phases
- **WHEN** a run failed or a phase was reused from the cache
- **THEN** its duration does not enter the figures

### Requirement: The estimated time of a project says where it comes from
The estimated time of a project SHALL be built from the phases it will run, using the history where there is enough and the measured table where there is not, and SHALL state which of the two it used and, for the history, how many executions it rests on.

#### Scenario: all from history
- **WHEN** every phase the project will run has enough history
- **THEN** the estimate is built from the history and says so

#### Scenario: mixed
- **WHEN** only some phases have enough history
- **THEN** the estimate uses the history for those and the table for the rest, and says it is mixed

#### Scenario: phases the cache will skip
- **WHEN** a phase will be reused from the cache
- **THEN** it adds nothing to the estimate

### Requirement: The statistics can be limited to a client and grouped by client

`GET /api/stats` SHALL accept `client`, one or more clients separated by commas, and keep only the runs of those clients; a value that is not a client name (one to 32 lowercase letters, digits or hyphens) SHALL be `400`. The client of a run is the one its generation or edit was requested with, `unknown` for one that was requested without or before clients existed, and `embedded` for the runs of the projects of the dashboard itself. The answer SHALL carry `by_client`, one group per client with its runs, seconds and success rate. Without `client` every run is counted. The generations of the API keep counting in the totals, the kinds and the GPUs as before and still do not make a project of their own in the table of projects.

#### Scenario: one dashboard

- **WHEN** `client=video-studio` is asked
- **THEN** only the runs of the generations and edits requested by `video-studio` are counted and `by_client` has that single group

#### Scenario: the dashboard itself

- **WHEN** `client=embedded` is asked
- **THEN** only the runs of ordinary projects are counted

#### Scenario: several

- **WHEN** `client=video-studio,unknown` is asked
- **THEN** the runs of both are counted and `by_client` has two groups

#### Scenario: not a client name

- **WHEN** `client=Video Studio` is asked
- **THEN** the answer is `400`

#### Scenario: without the filter

- **WHEN** `/api/stats` is asked without `client`
- **THEN** every run is counted and `by_client` lists each client that has runs
