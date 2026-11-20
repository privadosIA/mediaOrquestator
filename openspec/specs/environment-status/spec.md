# environment-status Specification

## Purpose
Defines the environment report the UI relies on: required binaries and their versions, available models/voices/LoRAs for editor dropdowns, GPU inventory with free VRAM, and the Ollama VRAM-retention caveat.

## Requirements

### Requirement: Binary presence and versions are reported

The server SHALL report, for `piper`, `sd`, `ffmpeg`, `ffprobe` and `nvidia-smi`: whether the binary is resolvable (using the same resolution the pipeline uses, including the project-local `bin/`), its reported version when obtainable, and — when missing — which `scripts/` setup script provides it. The report SHALL NOT offer any way to execute those scripts.

#### Scenario: missing binary names its setup script

- **WHEN** `sd` is not resolvable
- **THEN** the report marks it missing and names `scripts/build-sd.sh` as the fix

#### Scenario: optional binary absent is not an error

- **WHEN** `nvidia-smi` is not resolvable
- **THEN** the report marks it missing without failing, and GPU information is reported as unavailable

### Requirement: Models, voices and LoRAs are inventoried for the editor

The server SHALL list the model checkpoints under `models/`, the voices under `voices/`, and the LoRA files under the configured LoRA directory, each with name and file size, so the editor can populate its dropdowns. An empty or missing directory SHALL yield an empty list, not an error.

#### Scenario: populated inventories

- **WHEN** `models/` and `voices/` contain files
- **THEN** the report lists each file with its name and size

### Requirement: GPUs are listed with free VRAM

When `nvidia-smi` is available, the server SHALL report each GPU's index, name, and current free VRAM, refreshed on each request, so the UI can choose a render GPU and warn about low memory.

#### Scenario: two GPUs reported

- **WHEN** the machine has two NVIDIA GPUs and `nvidia-smi` works
- **THEN** the report lists both with index, name and free VRAM in MB

### Requirement: The Ollama caveat is surfaced when it is relevant

When an Ollama process is detected holding memory on a GPU, the environment report SHALL include an explicit warning for that GPU describing the `OLLAMA_KEEP_ALIVE` VRAM-retention caveat, so a user choosing that GPU for rendering is warned before launching.

#### Scenario: Ollama on the render candidate GPU

- **WHEN** Ollama is detected using memory on a GPU
- **THEN** the report attaches the caveat warning to that GPU's entry

### Requirement: The report states whether download keys are configured

The environment report SHALL include `tokens`, an object with a boolean per service, `huggingface` and `civitai`, that is `true` when the server has a non-empty key for that service (`HF_TOKEN`; `CIVITAI_TOKEN` or the Civitai key file) and `false` otherwise. The report SHALL NEVER include a key's value, whether it came from the environment or from a file, or the path of a key file. The values SHALL be read when the report is built, so a key added to the Civitai file without restarting the server shows as configured on the next request.

#### Scenario: no keys
- **WHEN** neither `HF_TOKEN` nor a Civitai key is configured
- **THEN** `tokens` is `{"huggingface": false, "civitai": false}`

#### Scenario: one key configured
- **WHEN** `HF_TOKEN` is set and no Civitai key exists
- **THEN** `tokens.huggingface` is `true` and `tokens.civitai` is `false`

#### Scenario: the Civitai key file is picked up without a restart
- **WHEN** the Civitai key file is created after the server started
- **THEN** the next report shows `tokens.civitai` as `true`

#### Scenario: nothing secret leaves the server
- **WHEN** keys are configured
- **THEN** the serialized report contains neither a key value nor a key file path

### Requirement: The report states where each configured key comes from

The environment report SHALL include `token_sources`, an object with `huggingface` and `civitai`, each `"env"` when the key comes from an environment variable, `"file"` when it comes from the key file, and omitted when no key is configured. When both exist, the source SHALL be `"env"`, which is the one in use. `tokens` SHALL stay as it is. The report SHALL NEVER include a key's value or the path of a key file.

#### Scenario: a key from the file
- **WHEN** only the Civitai key file exists
- **THEN** `token_sources.civitai` is `"file"` and `tokens.civitai` is `true`

#### Scenario: the environment wins
- **WHEN** `HF_TOKEN` is set and the Hugging Face file also exists
- **THEN** `token_sources.huggingface` is `"env"`

#### Scenario: no key
- **WHEN** no key is configured for a service
- **THEN** that service is absent from `token_sources`

#### Scenario: saving is reflected on the next report
- **WHEN** a key is saved through the API
- **THEN** the next report shows it as configured with source `"file"`

### Requirement: The music engine binaries are reported

The environment report SHALL include `ace-lm` and `ace-synth` with the same fields as `sd` and `piper` (presence, version when obtainable, an install hint pointing at `scripts/build-acestep.sh`). Their absence SHALL NOT affect the readiness of video or images projects. The installed music models SHALL be inventoried together with the other models.

#### Scenario: Engine missing
- **WHEN** `ace-synth` is not on `PATH`
- **THEN** the report shows it as missing with the install hint

#### Scenario: Video project unaffected
- **WHEN** the music engine is missing and a video project is run
- **THEN** the run is not blocked by it

### Requirement: Installed music adapters are reported

The environment report SHALL list the installed music adapters (one entry per file or folder, with its size) so that editors can offer them.

#### Scenario: Adapters in the report
- **WHEN** an adapter is installed
- **THEN** the report lists it with its size

### Requirement: The server program is reported and required by its engine

The environment report SHALL list `ace-server` among the optional music programs, and a project with `engine: server` SHALL fail pre-flight when it is missing, without requiring `ace-lm` or `ace-synth`.

#### Scenario: Reported
- **WHEN** the environment is read
- **THEN** `ace-server` appears with its presence

#### Scenario: Pre-flight
- **WHEN** a project has `engine: server` and `ace-server` is missing
- **THEN** the run is refused naming it

### Requirement: yt-dlp is an optional program

The environment report SHALL list `yt-dlp` among the optional programs with the same fields as the others (presence, version when obtainable, an install hint pointing at `scripts/get-ytdlp.sh`). Its absence SHALL NOT affect the readiness of any project: it is only needed to add a source from a YouTube URL.

#### Scenario: Absent
- **WHEN** `yt-dlp` is not installed
- **THEN** the report lists it as absent with the hint `bash scripts/get-ytdlp.sh` and the environment stays ready

#### Scenario: Installed
- **WHEN** `bin/yt-dlp` exists
- **THEN** the report shows its path and version

### Requirement: GPUs report Ollama's memory and low free VRAM

Each GPU of the report SHALL carry `ollama_mb`, the memory an Ollama process holds on it (absent when none), and `low_vram` when less than 6 GB is free. The server SHALL offer `POST /api/environment/ollama/unload`, which asks Ollama to drop every loaded model and answers their names (an empty list when none; `502` when Ollama cannot be reached).

#### Scenario: Ollama holds memory
- **WHEN** an Ollama process uses 4096 MB of a GPU
- **THEN** that GPU reports `ollama_mb` 4096

#### Scenario: Little free
- **WHEN** a GPU has 3123 MB free
- **THEN** it reports `low_vram`

#### Scenario: Unload
- **WHEN** a client posts to the unload endpoint while a model is loaded
- **THEN** the model is unloaded and its name is returned

### Requirement: The report states the machine's memory

The report SHALL carry `ram` with the machine's `total_bytes` and `available_bytes` and SHALL omit it when the memory cannot be read.

#### Scenario: Memory is read
- **WHEN** the report is requested on a machine whose memory can be read
- **THEN** `ram.available_bytes` is above 0 and not above `ram.total_bytes`

### Requirement: Each GPU reports its total memory

Besides the free VRAM, each GPU of the environment report SHALL carry `total_vram_mb`, read with `nvidia-smi` in the same query, so that what a model has to fit in can be told from what the desktop holds at the moment.

#### Scenario: Total and free
- **WHEN** a card has 16311 MB of which 1400 are free
- **THEN** the report says `free_vram_mb` 1400 and `total_vram_mb` 16311

### Requirement: Each GPU reports the speed measured in its runs

After a run, the server SHALL compare each render and animate phase it executed with the time the catalog expects at those settings on the reference machine, and store the ratio as a sample of the card the run used (the card its single GPU option names, or the machine's only card; a run on several cards, or whose card cannot be read, SHALL add nothing). Phases whose expected time is under 2 seconds, or whose ratio falls outside 0.15 to 25, SHALL be skipped. At most 60 samples of a card SHALL be kept. The environment report SHALL give a card `speed_factor`, the median of its latest 20 samples, and `speed_samples`, once it has at least 2.

#### Scenario: A card is calibrated
- **WHEN** a run of three renders on a card finishes
- **THEN** the report gives that card a `speed_factor` close to the ratio of the real times to the expected ones and `speed_samples` 3

#### Scenario: Not enough samples
- **WHEN** a card has fewer than 2 samples
- **THEN** its entry has no `speed_factor`

### Requirement: The report says which Ollama it talks to

The environment report SHALL include `ollama`, an object with `host` (the address in use), `source` (`setting`, `env` or `default`) and `remote` (whether it is not this machine), resolved when the report is built and without contacting Ollama. When `remote` is true the report SHALL NOT attach the local-GPU Ollama VRAM-retention warning, since a process on another machine does not hold this machine's memory, and `POST /api/environment/ollama/unload` SHALL act on the same address.

#### Scenario: the address in the report
- **WHEN** the saved setting is `http://192.168.1.50:11434`
- **THEN** the report has `ollama.host` `http://192.168.1.50:11434`, `source` `setting` and `remote` true

#### Scenario: no local warning for a remote Ollama
- **WHEN** Ollama is remote and a local process named ollama holds GPU memory
- **THEN** the GPU's warnings do not include the Ollama retention caveat

#### Scenario: unload uses the same address
- **WHEN** Ollama is unloaded from the environment screen
- **THEN** the request goes to the address in use

### Requirement: The environment reports the HeartMuLa engine

The environment report SHALL list, as an optional piece, the HeartMuLa engine (service installed, weights present, service reachable) with what is missing and how to fix it, and SHALL NOT list Docker. A missing optional piece SHALL not make the environment report as not ready.

#### Scenario: Ready
- **WHEN** the service and the weights are there
- **THEN** the engine is listed as ready

#### Scenario: Optional
- **WHEN** the engine is not installed
- **THEN** it is listed with how to install it and the environment is still ready

### Requirement: The environment report says the content mode

`GET /api/environment` SHALL carry `content_mode`: `censored` or `strict`. It describes the process: it does not change while the server runs.

#### Scenario: the default
- **WHEN** the server was started without flags
- **THEN** `content_mode` is `censored`

### Requirement: whisper-cli and its model are optional

The report SHALL list `whisper-cli` among the binaries with its presence, version and fixing command (`bash scripts/get-whisper.sh`), marked optional like `yt-dlp`: its absence SHALL never block a project of any mode. The report SHALL also say whether a Whisper model is installed (and its name). The program SHALL be looked up first in `bin/` and then in the `PATH`, like `sd`.

#### Scenario: Not installed
- **WHEN** neither `bin/whisper-cli` nor one in the `PATH` exists
- **THEN** the report says it is absent with the command that installs it and the lyrics extraction is the only thing unavailable

#### Scenario: The system one is found
- **WHEN** only `/usr/bin/whisper-cli` exists and a model is installed
- **THEN** the report shows it present with its path and the extraction is available

### Requirement: The environment says whether dictation can work

The environment report SHALL state whether dictation is available (Whisper, its model and ffmpeg present), so that an interface can hide the microphone when it could not work.

#### Scenario: nothing installed

- **WHEN** the environment is read on a machine without the Whisper model
- **THEN** it says dictation is not available and why

### Requirement: Retouch model files are inventoried apart

The environment report SHALL list in `retouch` the installed files of `models/` that belong only to retouch models of the catalog (the editing models and the encoders, projector and VAE they use), and `models` SHALL NOT list them. A file that an image model of the catalog also uses SHALL stay in `models`. Both lists SHALL be empty lists, not absent, when they have no files.

#### Scenario: retouch files leave the models

- **WHEN** `models/` holds an image checkpoint and the files of Qwen-Image-Edit
- **THEN** `models` lists the checkpoint and `retouch` lists the Qwen-Image-Edit files

#### Scenario: nothing installed

- **WHEN** `models/` has no files
- **THEN** `models` and `retouch` are both `[]`

### Requirement: The Blender engine is reported

The environment status SHALL report the Blender engine: whether it is available, its version, the Cycles devices it can use (NVIDIA cards and CPU) and, when it is not available, the reason (not installed, too old, no usable device).

#### Scenario: Available
- **WHEN** Blender 5.2 is installed and an NVIDIA card is present
- **THEN** the status shows it available with its version and the card

#### Scenario: Missing
- **WHEN** Blender is not installed
- **THEN** the status shows it unavailable with the reason "not installed"

### Requirement: demucs and its model are optional

The report SHALL list `demucs` among the binaries with its presence, version (the first line it prints with no arguments, or `unknown`) and fixing command (`bash scripts/get-demucs.sh`), marked optional like `whisper-cli`: its absence SHALL never block a project of any mode. The report SHALL also say whether a Demucs model is installed in `models/demucs/` (and its name). The program SHALL be looked up first in `bin/` and then in the `PATH`. The models inventory SHALL list the Demucs model next to the Whisper one, with the note that it separates the voice of a song for the timing of its words.

#### Scenario: Not installed
- **WHEN** neither `bin/demucs` nor one in the `PATH` exists
- **THEN** the report says it is absent with the command that installs it, and only the separation of the voice for the words is unavailable

#### Scenario: Installed
- **WHEN** `bin/demucs` and a model exist
- **THEN** the report shows them present with their paths
