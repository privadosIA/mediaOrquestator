# run-event-streaming Specification

## Purpose
Defines how a run's pipeline events reach HTTP clients over Server-Sent Events: the JSON-lines contract of the `events` capability is re-emitted verbatim, live and as replay, never altered or extended in place.

## Requirements

### Requirement: Pipeline events are re-emitted over SSE unaltered

The server SHALL expose an SSE stream per run in which every pipeline event is delivered as one SSE message whose data is the original JSON object from the pipeline's JSON-lines stream, unaltered: no fields added, removed, renamed or re-typed, and field-presence semantics preserved. Server-originated notifications (queue position, run state changes, stream end) SHALL use distinct SSE event types and SHALL never be merged into pipeline event objects.

#### Scenario: event payload is verbatim

- **WHEN** the pipeline emits a `phase_end` event line during a run
- **THEN** an SSE message delivers a JSON object identical to that line

#### Scenario: server notifications are separate

- **WHEN** the server reports a run's queue position or state change
- **THEN** that message uses a distinct SSE event type and no pipeline event object is modified to carry it

### Requirement: Subscribers get full replay then live tail

A client subscribing to an active run's stream SHALL first receive every event already emitted by that run, in order, then live events as they occur. Reconnecting mid-run SHALL therefore never lose events. Subscribing to a queued run SHALL deliver events once the run starts.

#### Scenario: late subscriber

- **WHEN** a client subscribes after several events were emitted
- **THEN** it receives those events in order before any newer event

### Requirement: Streams end explicitly after a terminal event

After a run reaches a terminal state — the pipeline's `pipeline_end` or `error` event, or a cancellation — the server SHALL send an explicit end-of-stream notification (as a server-originated SSE event) and close the stream, so clients can distinguish completion from a dropped connection.

#### Scenario: completed run closes the stream

- **WHEN** the pipeline emits `pipeline_end`
- **THEN** the subscriber receives that event, then an end-of-stream notification, then the connection closes

### Requirement: Historical runs replay over the same stream shape

Subscribing to a finished run's stream SHALL replay its stored events in order with the same message format as a live stream, followed by the end-of-stream notification, so clients use one consumer for live and historical runs.

#### Scenario: replay of a finished run

- **WHEN** a client subscribes to a run that already finished
- **THEN** it receives the full stored event sequence verbatim and then the end-of-stream notification
