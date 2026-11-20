# music-containers Specification

## Purpose
The song generators (HeartMuLa and DiffRhythm2) and the services beside them (LAION-CLAP, video vision) run as .NET services started and stopped by the user, and stay up between songs. The two generators can also run, as the person chooses, as the Python containers of `docker/` (see `music-docker-option`). This capability is the state of those services, the calls that start and stop them, and what a run does when its service is up or is not. (The name `container` of the state in the API is kept for compatibility; it is the state of the .NET service or, with the `docker` backend, of the Docker container.)

## Requirements

### Requirement: The engines run as .NET services

HeartMuLa, DiffRhythm2 and LAION-CLAP SHALL run as processes of this machine that run the service published in `bin/services/<name>` (by `scripts/get-services.sh`). A service that is not published, or without `dotnet`, SHALL be unavailable with the reason `no_service`, and `POST /api/music/engines/{engine}/start` SHALL answer `422` with it. DiffRhythm2's weights SHALL be the converted files (`decoder.safetensors`, `mulan_text.safetensors`, `mulan_text_tokenizer.json`). An engine MAY instead be an external service at an address saved in the settings.

#### Scenario: Start and stop

- **WHEN** the user starts `diffrhythm2` on the GPU 1 and later stops it
- **THEN** a process runs the service on that card, the state goes `starting` then `running`, and after the stop the process and its video memory are gone

#### Scenario: Not installed

- **WHEN** `bin/services/lionclap` does not exist
- **THEN** the engine is unavailable with `no_service` and the start is refused with `422`

### Requirement: The state of each service is reported

`GET /api/music/engines` SHALL carry, for each of the engines `heartmula` and `diffrhythm2` whose engine is local, `container: {state, gpu?, port?, since?, memory_mb?}` where `state` is `stopped`, `starting` or `running`; `gpu` is the index of the card it was given, `port` the port it listens on at `127.0.0.1`, `since` the time it started and `memory_mb` the memory its process uses (with `?stats=1`). It SHALL start nothing, and be kept 30 seconds like the rest of the answer, dropped when a service is started or stopped. An engine whose address is another machine's has no `container`.

#### Scenario: Running
- **WHEN** the service of `diffrhythm2` runs on the GPU 1
- **THEN** its `container` says `running`, `gpu` 1 and the port

#### Scenario: Both at once
- **WHEN** both services run
- **THEN** both are reported `running` with their own ports

### Requirement: A service is started and stopped by the user

`POST /api/music/engines/{engine}/start` with an optional `{"gpu": N}` SHALL start the service of `heartmula` or `diffrhythm2` on the card asked for, or on the one with most free memory among those that have what the engine needs, listening on `127.0.0.1` only, answer `202` at once and wait for the service to say ready in the background; the state is `starting` meanwhile and `running` once it is ready, or `stopped` with the reason in `container.error` when it did not get ready. With the `docker` backend of the generator (see `music-docker-option`) it SHALL run the container of the generator instead of the .NET service, with the same states and the same answers. It SHALL answer `409` when the service is already running or starting, or when the generator is an address typed by hand (there is no local service to start), `422` with the reason (`no_service`, `no_weights`, `no_gpu`, and for the container `no_docker`, `no_image`, `no_gpu_runtime`) when it cannot be started, and `404` for another engine. `POST /api/music/engines/{engine}/stop` SHALL end the service and answer `200`, `409` when a run that uses that generator is going, and `404` for another engine.

#### Scenario: Start
- **WHEN** the user starts `diffrhythm2` with `gpu: 1`
- **THEN** the answer is `202`, the state is `starting` and then `running`, and the service is on the GPU 1

#### Scenario: Already up
- **WHEN** the service is running and it is started again
- **THEN** the answer is `409`

#### Scenario: No room
- **WHEN** no card has the memory the engine needs
- **THEN** the answer is `422` with `no_gpu` and says how much each card has free

#### Scenario: Stop during a run
- **WHEN** a run that makes songs with that generator is going and the service is stopped
- **THEN** the answer is `409` and the service keeps running

#### Scenario: The Docker backend
- **WHEN** the backend of `heartmula` is `docker`, its image and weights are there, and it is started with `gpu: 1`
- **THEN** the answer is `202`, the container runs on the GPU 1, `container` says `running` with that card and the port of its image, and stopping it removes the container

#### Scenario: An address typed by hand
- **WHEN** `heartmula_host` is saved and the generator is started
- **THEN** the answer is `409` whatever its backend is

### Requirement: A service that was started stays up

No run SHALL stop a service that the user started, and stopping the application SHALL NOT stop it: the user stops it from the Environment screen. A reboot of the machine leaves the services stopped. Both engines MAY run at the same time, each on its own card or on the same one when it has the memory.

#### Scenario: A run ends
- **WHEN** a run that used a running service ends
- **THEN** the service is still running

#### Scenario: The application stops
- **WHEN** the server is stopped
- **THEN** the services it did not start itself are still running

### Requirement: A run made through the server does not start services

When the generator of a project is `heartmula` or `diffrhythm2`, its engine is local (no address of another machine is saved) and its service is not running, `POST /api/projects/{id}/runs` SHALL refuse the run with `422` and the code `engine_not_running`, naming the generator and saying that it is started from the Environment screen, before anything is queued. A run already queued that finds the service stopped when it starts SHALL fail the same way with the reason `not_running`. The command line without a server SHALL keep starting the service for the run and stopping it when the run ends; a service that was already running when a run started SHALL be left running in both cases.

#### Scenario: Stopped
- **WHEN** a run is launched on a project of `diffrhythm2` and its service is stopped
- **THEN** the answer is `422 engine_not_running` and no run is queued

#### Scenario: Running
- **WHEN** the service is running
- **THEN** the run is queued and uses it

#### Scenario: External
- **WHEN** an address of another machine is saved for the engine
- **THEN** the run does not need a local service

#### Scenario: The command line
- **WHEN** a run is made with the command line and no service is running
- **THEN** the service is started for the run and stopped when it ends

### Requirement: The availability says when the engine only needs its service started

The availability of a local engine SHALL carry the reason `not_running` when the service is installed, the weights are complete and a card is big enough, but the service is stopped; it SHALL be `available: false` for a run made through the server and say so. The music studio SHALL keep offering the generator in that case, with a notice that its service is stopped and a link to the Environment screen, and SHALL disable the button that makes the songs while the project uses it.

#### Scenario: Stopped service
- **WHEN** everything is installed and the service is stopped
- **THEN** the engine reports `not_running`, the selector still offers it with the notice, and creating songs is disabled with the link

#### Scenario: Started
- **WHEN** the service becomes `running`
- **THEN** the engine is available, the notice goes and creating is enabled

### Requirement: DiffRhythm2 keeps its model in RAM between songs

The DiffRhythm2 service SHALL keep its model, its decoder and its style encoder in RAM while it is idle, move them to the GPU only while it makes a song, and put them back and empty the GPU cache when the song ends, so that a service that is up and idle holds no video memory. The first song after the service starts SHALL load them; the following ones SHALL not read the weights from disk again. The memory at rest (RAM and VRAM) of each service SHALL be measured on this machine and written in the documentation.

#### Scenario: Idle
- **WHEN** the service has made a song and is idle
- **THEN** the video memory it holds is only the CUDA context

#### Scenario: Second song
- **WHEN** a second song is asked for
- **THEN** it starts without reading the weights from disk

### Requirement: A started service survives the server

A service SHALL be started in its own session and its pid, port and card SHALL be kept in a file next to the database, so that stopping or restarting the server does not stop it and the new server finds it running. A pid in that file that is no longer the service (the process ended, or the number was reused) SHALL be treated as stopped and SHALL NOT be signalled.

#### Scenario: The server restarts

- **WHEN** a service is running and the server is restarted
- **THEN** the engine is reported `running` on the same port

#### Scenario: A reused pid

- **WHEN** the file names a pid that now belongs to another program
- **THEN** the engine is `stopped` and that program is left alone

### Requirement: Starting lasts until the service is ready

While a start waits for `/health` to say ready, the engine SHALL be `starting` and unavailable, even though its process already exists.

#### Scenario: Loading the model

- **WHEN** the process of HeartMuLa is up and its model is still loading
- **THEN** the state is `starting` and a run is refused with `engine_not_running`

### Requirement: HeartMuLa's codec goes to another card when there is one

For HeartMuLa the model SHALL be placed on the card chosen and the codec on the other card with most free memory, or on the same one when the machine has a single card.

#### Scenario: Two cards

- **WHEN** HeartMuLa is started on the GPU 0 of a machine with two cards
- **THEN** its model is on the GPU 0 and its codec on the GPU 1
