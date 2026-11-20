# run-control Specification

## Purpose
Defines how pipeline runs are launched, queued per GPU, cancelled, and durably recorded in the run history, without changing the pipeline's own semantics.

## Requirements

### Requirement: A run launches the existing pipeline with the CLI's options

The server SHALL launch a run for a project with options equivalent to the CLI flags `--gpu`, `--keep-tmp`, `--force`, `--only <scene-id>`, `--vram-warn-mb` and `--mode`, executing the existing pipeline unchanged and consuming its JSON-lines event stream. Launching SHALL validate the project first and reject the run (without queueing it) when validation fails. Each run SHALL get a unique identifier returned at launch.

#### Scenario: launch with options

- **WHEN** a run is launched for a valid project with a GPU index and `only` set to one scene id
- **THEN** a run id is returned and the pipeline executes for that project with those options

#### Scenario: invalid project never queues

- **WHEN** a run is launched for a project whose `scenes.yaml` fails validation
- **THEN** the response carries the validation error and no run is queued or recorded as started

### Requirement: At most one active run per GPU, FIFO otherwise

The server SHALL run at most one pipeline at a time per GPU index (runs with no GPU index count as one shared lane), queueing further launches in FIFO order per lane. A queued run SHALL report its state and queue position; it starts automatically when its lane frees up.

#### Scenario: second run on the same GPU queues

- **WHEN** a run is active on GPU 1 and another run is launched for GPU 1
- **THEN** the second run is queued, reports its position, and starts only after the first finishes

#### Scenario: different GPUs run concurrently

- **WHEN** a run is active on GPU 1 and another run is launched for GPU 0
- **THEN** both runs execute concurrently

### Requirement: Runs can be cancelled whether queued or active

The server SHALL cancel a queued run by removing it from the queue, and an active run by terminating its pipeline subprocesses. Either way the run SHALL be recorded with a cancelled state and SHALL free its lane for the next queued run. Cancelling an already finished run SHALL be rejected.

#### Scenario: cancel active run

- **WHEN** an active run is cancelled
- **THEN** its subprocesses terminate, its state becomes cancelled, and the next queued run on that lane starts

#### Scenario: cancel queued run

- **WHEN** a queued run is cancelled
- **THEN** it leaves the queue without ever starting and is recorded as cancelled

### Requirement: Run history is durable and complete

Every run SHALL be durably recorded with its project, options, state (queued, running, succeeded, failed, cancelled, interrupted), timestamps, final artifact path when one was produced, and its full event stream stored verbatim (each event exactly as emitted). History SHALL survive a server restart; runs left in a non-terminal state by a crash or shutdown SHALL be marked interrupted on the next startup. History SHALL be listable (most recent first, filterable by project) and each run's detail retrievable by id.

#### Scenario: history survives restart

- **WHEN** the server restarts after completed runs
- **THEN** listing history returns those runs with their states, options and timestamps intact

#### Scenario: crash marks runs interrupted

- **WHEN** the server starts and finds runs recorded as queued or running from a previous process
- **THEN** those runs are marked interrupted

### Requirement: A run with several GPUs occupies all their lanes

A run launched with several GPU indexes SHALL occupy every one of those lanes while it is active. It SHALL start only when none of its lanes has an active run, and runs SHALL stay in FIFO order per lane: a queued run SHALL NOT start while an earlier queued run shares one of its lanes. The run's GPU option SHALL be validated like the CLI's (distinct non-negative integers, comma-separated) and reported as given.

#### Scenario: multi-GPU run waits for a busy lane
- **WHEN** a run is active on GPU 1 and a run for GPUs `0,1` is launched
- **THEN** the second run is queued and starts only after the first finishes

#### Scenario: a single-GPU run behind a multi-GPU run
- **WHEN** a queued run for GPUs `0,1` is waiting and a run for GPU 0 is launched
- **THEN** the run for GPU 0 queues behind it

#### Scenario: invalid GPU list is rejected at launch
- **WHEN** a run is launched with `gpu: "0,0"`
- **THEN** the launch is rejected and nothing is queued

### Requirement: Finished runs can be deleted from the history

The server SHALL delete runs from the history one at a time (`DELETE /api/runs/{id}`) and several at once (`POST /api/runs/delete` with either `ids` or a filter of `project`, `states` and `before`, combined with AND; `ids` together with a filter is a `400`). Deleting a run SHALL remove the run and all its stored events in one transaction, so no run is ever left without its events or the reverse. Only runs in a terminal state (`succeeded`, `failed`, `cancelled`, `interrupted`) SHALL be deleted: a run that is queued or running, or that the run manager still holds as live, SHALL be refused with `409` saying it must be cancelled first, and in a batch it SHALL be skipped and listed in `skipped_active` while the rest are deleted. A filter naming no states SHALL still never delete an active run. A batch answers `{deleted, skipped_active}`; an id that does not exist is `404` when single and ignored in a batch; a filter that matches nothing answers `deleted: 0`. Deleting SHALL NOT touch any file a run produced.

#### Scenario: deleting one finished run
- **WHEN** a succeeded run is deleted by id
- **THEN** the answer is `204`, the run and its events are gone from the history and its output files are still on disk

#### Scenario: an active run is refused
- **WHEN** a running or queued run is deleted by id
- **THEN** the answer is `409` telling the user to cancel it first, and the run and its events are unchanged

#### Scenario: a batch skips active runs
- **WHEN** a batch names two finished runs and one running run
- **THEN** the two finished runs are deleted, the running one is listed in `skipped_active` and the answer is `{deleted: 2, ...}`

#### Scenario: delete by filter
- **WHEN** `project` is `test` and `states` is `["failed"]`
- **THEN** only the failed runs of that project are deleted

#### Scenario: delete before a date
- **WHEN** `before` is a timestamp
- **THEN** only terminal runs created before it are deleted

#### Scenario: nothing matches
- **WHEN** a filter matches no run
- **THEN** the answer is `deleted: 0` and nothing changes

#### Scenario: ids and a filter together
- **WHEN** a request carries both `ids` and a filter
- **THEN** the answer is `400` and nothing is deleted

#### Scenario: a failure leaves no half-deleted run
- **WHEN** removing the events fails
- **THEN** the run is still in the history with all its events

### Requirement: A deleted run is gone for every endpoint

After a run is deleted, `GET /api/runs/{id}`, its events and its stream SHALL answer `404`, and it SHALL not appear in the history list.

#### Scenario: fetching a deleted run
- **WHEN** a deleted run's id is requested
- **THEN** the answer is `404` with the standard error body

### Requirement: Deleting a run also deletes its resource samples

Deleting a run from the history SHALL remove the resource samples stored for it in the same transaction as the run and its events, so no sample outlives its run.

#### Scenario: samples go with the run
- **WHEN** a run with samples is deleted
- **THEN** its samples are gone together with the run and its events

#### Scenario: failure leaves everything
- **WHEN** removing the samples fails
- **THEN** the run, its events and its samples are all still there

### Requirement: A run without a GPU uses the card with the most memory

When a run is launched without `gpu` and the environment reports two or more GPUs, the server SHALL pin it to the GPU with the most total memory (the free memory only when the total is unknown), so that `sd` does not fall on the first card, which usually holds the desktop and has the least memory. The run record SHALL show the GPU used. With a single GPU or when the GPUs cannot be read (no `nvidia-smi`), no GPU SHALL be pinned. A `gpu` given by the caller, one index or a list, SHALL be respected.

#### Scenario: Two cards
- **WHEN** a run is launched without `gpu` on a machine with a 12 GB and a 16 GB card
- **THEN** the run is on the 16 GB card and its record says so

#### Scenario: Chosen explicitly
- **WHEN** a run is launched with `gpu` "0"
- **THEN** it runs on GPU 0

#### Scenario: Cards cannot be read
- **WHEN** `nvidia-smi` is not available
- **THEN** nothing is pinned

### Requirement: Music runs use every card that fits a song

When a run is launched without a GPU, for a music project and with two or more songs to make (all of them, or one when the run is restricted with `only`), the server SHALL pin the run to every card with at least 9000 MB of total memory when there are two or more, so the songs are made at the same time. In any other case, or when the cards cannot be read, it SHALL keep the previous choice (the card with the most memory, or none). A GPU given in the request SHALL be respected.

#### Scenario: Two cards, several songs
- **WHEN** a music project with two songs is launched without a GPU on a machine with a 12 GB and a 16 GB card
- **THEN** the run is recorded with GPU `0,1`

#### Scenario: One song
- **WHEN** the run is restricted to one song
- **THEN** the run uses the card with the most memory

#### Scenario: A card too small
- **WHEN** one of the two cards has 4 GB
- **THEN** the run uses only the card that fits

### Requirement: The queue has a size

The server SHALL refuse to queue a run when the queue already holds a fixed maximum (at least 20 runs in all, and at least 5 for the same project), answering `429` with the standard error body and the code `queue_full`. The same rule SHALL apply to model downloads and to music-motion jobs with their own constants. Cancelling or finishing a run frees its place.

#### Scenario: a full queue
- **WHEN** the maximum number of runs is queued and another launch arrives
- **THEN** the answer is `429 queue_full` and the history has no new row

#### Scenario: the same project twice
- **WHEN** a project already has the per-project maximum of queued runs (5) and another launch arrives for it
- **THEN** the answer is `429 queue_full` naming the project
