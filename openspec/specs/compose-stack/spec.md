# compose-stack Specification

## Purpose
How the whole system starts from one Docker Compose file: the database, the server, its monitoring, the optional pieces and the host folders.

## Requirements

### Requirement: One file starts the whole system

The repository SHALL carry one Compose file, `compose.yaml` at its root, that defines every piece: the server, its MySQL database, the step that migrates the schema, Ollama, the HeartMuLa and DiffRhythm2 generators, Prometheus, Grafana, Loki and the shipper of the logs, and Redis. `docker compose up -d` with no profile SHALL start the server, the database, the migration, Prometheus and Grafana. Every other piece SHALL be a profile of the same file (`music`, `local-llm`, `logs`, `redis`, plus `install` for the installation step), so that a second Compose file is never needed. The OpenAI-compatible providers SHALL need nothing in the stack: they are an address and a key in Settings. The server SHALL answer on a published port once the stack is up, and `docker compose ps` SHALL show every started service as healthy.

#### Scenario: The default start

- **WHEN** `docker compose up -d` is run on a machine with Docker and an `.env` made from `.env.example`
- **THEN** the server, MySQL, Prometheus and Grafana run, the schema is migrated, and `GET /api/environment` of the server answers

#### Scenario: A profile adds a piece

- **WHEN** `docker compose --profile music up -d` is run
- **THEN** the two generators start beside the default pieces and the engines API lists them as available

#### Scenario: Stopping loses nothing

- **WHEN** the stack is stopped with `docker compose down` and started again
- **THEN** the projects, the database, the keys, the downloaded models and the dashboards are still there

### Requirement: MySQL is the only database server

The stack SHALL run one database server, MySQL, and the server SHALL keep all its state in it. Grafana SHALL keep its own state in a schema of that same server, with a user of its own, and no other piece of the public file SHALL need a database server: Prometheus and Loki keep their own storage in volumes. A second MySQL, a Postgres or a SQLite file of the server or of Grafana SHALL NOT be part of the stack.

#### Scenario: The server's state

- **WHEN** the stack is up and a project is created through the API
- **THEN** it is a row of the MySQL of the stack and no database file of the server exists in its volumes

#### Scenario: The other pieces

- **WHEN** the services of the stack are listed
- **THEN** the only one that is a database server is MySQL, and Grafana's state is in a schema of it

### Requirement: The schema is migrated before the server starts

A one-shot step of the stack SHALL run the migration of the schema against the MySQL once it is healthy, and the server SHALL start only after that step ended well. The server SHALL still never migrate by itself. A migration that fails SHALL leave the server not started and the step's log SHALL say why.

#### Scenario: A first start

- **WHEN** the stack starts with an empty MySQL
- **THEN** the migration step creates the tables, ends, and then the server starts

#### Scenario: A version with new migrations

- **WHEN** the server image is replaced by one that brings migrations and the stack is started
- **THEN** the step applies them before the new server starts

#### Scenario: A migration that fails

- **WHEN** the migration step fails
- **THEN** the server is not started and the step's log names the migration

### Requirement: The server starts in its default content mode

The server of the stack SHALL start in the content mode the server has by default, without any option added by the stack. Any other option of the server's command line SHALL be given only through one variable of the environment, empty by default, that the person sets in their own `.env`. The Compose file, the Dockerfile, `.env.example` and the documents of the stack that the public tree carries SHALL NOT name an option, a variable, a script or a piece that exists only in the private edition; the edition SHALL be chosen when the image is built, through a build argument that is empty by default.

#### Scenario: The default

- **WHEN** the stack is started with the example environment
- **THEN** `GET /api/environment` of the server reports the default content mode, and a project marked for adults does not exist for the API

#### Scenario: The public tree

- **WHEN** the public tree is produced
- **THEN** none of the files of the stack names an option of the private edition, a personal path, a host name or a private address of the owner

### Requirement: Secrets are given by the person and never written in the file

The passwords of the database and of Grafana and the token of the server SHALL come from the person's `.env` or from files given to the services as secrets; `compose.yaml` and `.env.example` SHALL hold no real secret. The token of the server SHALL be required for every call, as the server requires it on any address that is not loopback. No secret SHALL appear in the log of a service or in an answer of the API.

#### Scenario: An example environment

- **WHEN** `.env.example` is read
- **THEN** every password and token in it is a placeholder that the stack refuses to start with until it is replaced

#### Scenario: A call with no token

- **WHEN** the API of the server of the stack is called without its token
- **THEN** the answer is `401`

### Requirement: Only what the person needs is published, and on the loopback by default

The server SHALL be published on a port of the host and Grafana on another; every other service SHALL be reachable only on the private network of the stack. The published ports SHALL bind to 127.0.0.1 unless the person sets another address in `.env`.

#### Scenario: The default binding

- **WHEN** the stack is up on the default settings
- **THEN** the server and Grafana answer on 127.0.0.1 and nothing answers on the other addresses of the host

#### Scenario: A service of the stack

- **WHEN** a service other than the server and Grafana is looked up on the host's ports
- **THEN** it is not published

### Requirement: Every piece starts configured with what it needs of the others

Nothing SHALL have to be typed in a screen or edited in a file for the pieces of the stack to work together. The server SHALL find Ollama, the HeartMuLa and DiffRhythm2 generators and the monitoring tools (Grafana, Prometheus, Loki and Redis) at their addresses in the stack, with their keys where they have one, taking them from the environment with the rule the Ollama address already has: what is saved in Settings wins, the environment is the default when nothing is saved, and the default is empty for a piece that is not part of the start. Prometheus SHALL read the metrics of the server with its token, Grafana SHALL find its data sources and its database, and the shipper SHALL write to Loki, all given by the Compose file. The server SHALL work, with the tasks that need a piece reported unavailable, when a profile is not started.

#### Scenario: A started piece

- **WHEN** the `music` profile is started and nothing is saved in Settings
- **THEN** the engines API lists HeartMuLa and DiffRhythm2 with the addresses of the containers

#### Scenario: The Monitoring card

- **WHEN** the stack is up for the first time and Settings are opened
- **THEN** the Monitoring card holds the addresses of Prometheus and Grafana and the test of each answers ok

#### Scenario: What is saved wins

- **WHEN** another HeartMuLa address is saved in Settings
- **THEN** the server uses the saved one

#### Scenario: A piece that is not started

- **WHEN** the stack runs without the `music` profile
- **THEN** the music tasks that need a generator are reported unavailable with the reason and the server answers every other call

### Requirement: The dashboard stays outside and connects to the server

The dashboard (`dashboards-lab`) SHALL NOT be part of the stack. The server SHALL be published for it, and the origin the dashboard is served from SHALL be given in `.env` and allowed as a browser origin when the server starts, so that no command has to be run on the container; the token of the server SHALL be the way the dashboard authenticates, and the documentation SHALL say how to read it and which address to give the dashboard.

#### Scenario: A dashboard on another origin

- **WHEN** the origin of the dashboard is set in `.env` and the stack is started
- **THEN** a browser page of that origin can call the API with the token, and a page of another origin is refused

#### Scenario: No dashboard origin

- **WHEN** the variable is empty
- **THEN** the server allows no browser origin besides its own

### Requirement: The pieces that use the GPU ask for it

The services that need a graphics card (the server, Ollama and the two generators) SHALL ask for it through the Compose device reservation, and the documentation SHALL say that the stack needs Docker with the NVIDIA container runtime and SHALL give the command that checks it before anything is started. The server SHALL report, through its environment, the card it sees and the tasks that need it.

#### Scenario: A machine with a card

- **WHEN** the stack starts on a machine with the NVIDIA container runtime
- **THEN** `GET /api/environment` lists the card and the tasks that need it are available

#### Scenario: A machine without the runtime

- **WHEN** the check command is run on a machine without it
- **THEN** it says what is missing and starts nothing

### Requirement: Prometheus and Grafana come configured

Prometheus SHALL scrape the metrics of the server with its token, and Grafana SHALL start with its data sources and both dashboards of `docs/grafana` already provisioned: the machine dashboard on Prometheus and the run-history dashboard on the MySQL, through a user that can read every table of the server's schema and cannot write anything. Grafana's own database and that read-only user SHALL be created and kept up to date from `.env` by a step of the stack that runs before Grafana starts and needs no table to exist. With the `logs` profile Loki SHALL be a data source too and SHALL receive the logs of the containers.

#### Scenario: The machine dashboard

- **WHEN** the stack has run for a minute and the machine dashboard is opened
- **THEN** it shows the runs by state and the memory of the machine with no step by hand

#### Scenario: The history dashboard

- **WHEN** a run has ended and the history dashboard is opened
- **THEN** it lists the run, and the user it reads with is refused any `INSERT`, `UPDATE`, `DELETE` or `DROP`

#### Scenario: Grafana's state

- **WHEN** a dashboard is saved in Grafana and the stack is restarted
- **THEN** it is still there, and no SQLite file of Grafana exists in its volumes

#### Scenario: A changed password

- **WHEN** a password is changed in `.env` and the stack is recreated (`docker compose up -d --force-recreate`)
- **THEN** the step updates the users and Grafana connects with the new one

#### Scenario: The logs

- **WHEN** the `logs` profile is started
- **THEN** the logs of the server are searchable in Grafana

### Requirement: The files live on the machine and survive an update

Everything the server keeps as files SHALL be in folders of the host machine given in `.env`, mounted into the containers, and never in a container's own layer: the projects and the media directory, the library items (the music-motion library of characters and backgrounds, the references, the control frames and previews of the motions), the models and voices, the keys and the token, and the installation elements (the programs the installer builds, the third-party runtime and libraries, and the HyperFrames blocks). Only the database and the data of Prometheus and Loki SHALL be Docker volumes. The images SHALL hold the program and its base libraries and nothing the person installs or creates. Replacing the images and starting again SHALL keep all of it, and each of those folders SHALL be placeable on its own disk through `.env`.

#### Scenario: A new image

- **WHEN** the server image is rebuilt and the stack is started
- **THEN** the projects, the library items, the keys, the downloaded models and the installed tools are as they were

#### Scenario: Another disk

- **WHEN** the folder of the models or of the projects is set to a path in `.env`
- **THEN** the server reads and writes its files there

#### Scenario: The files are on the host

- **WHEN** a project is created and a model is downloaded through the server of the stack
- **THEN** their files are in the host folders of `.env`, outside every container

### Requirement: The installation elements are installed by a step of the stack

A step of the same Compose file (profile `install`) SHALL run the pinned install scripts of the repository in an image with the build chain and write what they build or download into the host folders of the installation elements, so that the programs run in the image of the server. It SHALL be repeatable (what is installed is not built again) and SHALL say what is missing before it changes anything. The server SHALL start with whatever is installed and report each task whose element is missing as unavailable with the reason.

#### Scenario: A first installation

- **WHEN** `docker compose --profile install run --rm installer` is run on empty folders
- **THEN** the programs and the third-party pieces are in the host folders, and the server of the stack uses them

#### Scenario: Run again

- **WHEN** the step is run again with everything installed
- **THEN** it builds nothing and says so

#### Scenario: A missing element

- **WHEN** the server starts without a model or a program
- **THEN** the tasks that need it are reported unavailable with the reason and the rest work

### Requirement: The stack says what it leaves out

The Compose file, the README and the documentation of the stack SHALL state, in words that are true in both editions (the private edition adds its own sentence in a marked block), that the stack has no container for the .NET services (the song recognition and the vision of the video-edit tools), that the server runs them itself from the installation folder, that where the folder has none the tasks that need them are reported unavailable with their reason, and that the installer step of the public edition does not build them; and that the dashboard (`dashboards-lab`) is a separate project that connects from outside. The statement SHALL name no option, piece or file that exists only in another edition.

#### Scenario: What a reader finds

- **WHEN** a reader opens `compose.yaml`, the README or `docs/COMPOSE.md`
- **THEN** each says what the stack does not include and where the dashboard runs
