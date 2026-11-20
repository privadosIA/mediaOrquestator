# model-management Specification

## Purpose
Lets the management API inventory, install and remove the model checkpoints, Piper voices and LoRA files the pipeline reads from disk, including downloading them from a URL or Hugging Face with observable progress.

## Requirements

### Requirement: Installed assets are listed per kind

The server SHALL expose the installed assets for four kinds — `model` (directory `models/`), `voice` (`voices/`), `lora` (`models/loras/`) and `video` (`models/video/`) — each entry with name and size in bytes. Hidden files and `.partial` files SHALL NOT be listed as installed. A missing directory SHALL yield an empty list, not an error. Files under `models/video/` SHALL NOT appear in the `model` listing or in the environment inventory used by the editor.

#### Scenario: populated kinds

- **WHEN** `models/` holds a checkpoint and `voices/` holds an `.onnx` and its `.onnx.json`
- **THEN** the listing returns each file under its kind with its size

#### Scenario: missing directory

- **WHEN** `models/loras/` does not exist
- **THEN** the `lora` kind lists no entries and the response is successful

#### Scenario: video files stay out of the model list

- **WHEN** `models/video/` holds a `.gguf`
- **THEN** it is listed under the `video` kind and not under `model`

### Requirement: A recommended catalog is served

The server SHALL serve a built-in catalog of recommended assets, each with an id, kind, display name, a description stating in plain language what the asset is for, approximate size, source URL(s) and destination file name(s). Each file MAY carry a `role` (for example `model`, `text_encoder`, `vae`) and an entry MAY carry a `starter` flag marking it as a sensible first install. Entries of kind `model` SHALL also carry `vram_mb`, an approximate minimum GPU memory for rendering with them; entries of kind `video` MAY carry it when it was measured; entries of other kinds SHALL omit it. A voice entry SHALL include both its `.onnx` and `.onnx.json` files and a video entry SHALL list every file the model needs. The catalog SHALL include recommended entries for `model`, `voice`, `lora` and `video`. Each catalog entry SHALL be reported as installed when all its destination files exist. The catalog SHALL NOT depend on any remote search or listing service.

#### Scenario: catalog entry state

- **WHEN** the catalog is requested and `v1-5-pruned-emaonly.safetensors` is present in `models/`
- **THEN** the SD 1.5 entry is reported as installed and the others as not installed

#### Scenario: multi-file voice

- **WHEN** only the `.onnx` of a catalog voice exists
- **THEN** that entry is reported as not installed

#### Scenario: purpose and memory hints

- **WHEN** the catalog is requested
- **THEN** every entry has a non-empty description, every `model` entry has a positive `vram_mb`, and no `voice` or `lora` entry has one

#### Scenario: every kind has recommendations

- **WHEN** the catalog is requested
- **THEN** it contains at least one entry of each kind `model`, `voice`, `lora` and `video`

#### Scenario: video bundle lists its files

- **WHEN** a `video` entry is returned
- **THEN** its files include the diffusion model, the text encoder and the VAE, each with a role

### Requirement: Downloads run in the background with observable progress

The server SHALL start a download from a request naming a `kind` and either a direct `url` or a Hugging Face `repo` and `file` (with optional `revision`, default `main`), or a catalog `id`. It SHALL answer `202` with a download record (id, state, kind, name, source) without waiting for completion. Data SHALL be written to `<name>.partial` in the destination directory and renamed to the final name only after a complete, successful transfer. Progress (bytes received, total when known, state) SHALL be streamed over SSE with replay of past events on connect, ending with an explicit stream-end notification, following the same conventions as run events. States SHALL be `queued`, `running`, `succeeded`, `failed`, `cancelled`. At most one download SHALL run at a time; others SHALL queue in order.

#### Scenario: successful download

- **WHEN** a valid URL download is started and completes
- **THEN** progress events show increasing bytes, the final state is `succeeded`, and the file exists under its final name with no `.partial` left

#### Scenario: failed transfer leaves no asset

- **WHEN** the remote connection fails or answers a non-2xx status mid-way
- **THEN** the state is `failed` with the error message, the `.partial` file is removed, and no file under the final name exists

#### Scenario: Hugging Face source

- **WHEN** a download names `repo: "org/name"`, `file: "weights/model.safetensors"`
- **THEN** it fetches from that repository's `resolve/main/` path and installs the file under its base name

#### Scenario: queueing

- **WHEN** a second download is started while one is running
- **THEN** it is `queued` and starts only after the first reaches a terminal state

### Requirement: Downloads can be cancelled and are inspectable

The server SHALL list downloads (newest first, recent history kept in memory), return one by id, and cancel a queued or running download, removing its `.partial` file. Cancelling a terminal download SHALL answer `409`. Download records need not survive a server restart; on shutdown a running download SHALL be cancelled and its `.partial` removed.

#### Scenario: cancel running

- **WHEN** a running download is cancelled
- **THEN** its state becomes `cancelled` and its `.partial` file is gone

#### Scenario: cancel finished

- **WHEN** cancel is requested for a `succeeded` download
- **THEN** the response is `409` with the structured error body

### Requirement: Asset names and sources are validated

The asset name SHALL be a bare file name (derived from the URL path or `file` when not given): it SHALL NOT contain path separators, be empty, start with `.`, or equal `.`/`..`. Its extension SHALL belong to the allow-list for its kind (`model`: `.safetensors`, `.ckpt`, `.gguf`; `voice`: `.onnx`, `.onnx.json`; `lora`: `.safetensors`, `.ckpt`, `.gguf`; `video`: `.safetensors`, `.gguf`). The source scheme SHALL be `http` or `https`. A destination that already exists SHALL be refused with `409` unless the request sets `overwrite: true`. Violations SHALL answer `400`/`409`/`422` with the standard error body and start nothing.

#### Scenario: path traversal name

- **WHEN** a download requests the name `../../etc/passwd.safetensors`
- **THEN** the response is an error and nothing is written outside the destination directory

#### Scenario: disallowed extension

- **WHEN** a `model` download resolves to a name ending in `.exe`
- **THEN** it is rejected before any network request

#### Scenario: existing file

- **WHEN** the destination file already exists and `overwrite` is not set
- **THEN** the response is `409` and the existing file is untouched

#### Scenario: video accepts gguf

- **WHEN** a `video` download resolves to `model-Q4_K_S.gguf`
- **THEN** it is accepted, and a `.ckpt` name is rejected for that kind

### Requirement: Network targets are restricted

The server SHALL refuse to download from hosts that resolve to loopback, private, link-local or unspecified addresses, checked on connection so that redirects cannot reach them either, and SHALL follow a bounded number of redirects. An optional Hugging Face token from the server's environment (`HF_TOKEN`) SHALL be sent only to `huggingface.co` hosts and SHALL NOT appear in logs, records or responses. Before transferring, when the size is known and the free disk space is smaller, the download SHALL fail with a clear message.

#### Scenario: internal address

- **WHEN** a download URL points at `http://127.0.0.1:4666/api/health` or a host resolving to a private address
- **THEN** the download fails or is rejected without contacting that address

#### Scenario: insufficient disk space

- **WHEN** the announced size exceeds the free space on the destination filesystem
- **THEN** the state is `failed` with a message about disk space and no `.partial` remains

### Requirement: Assets can be deleted safely

The server SHALL delete a named installed asset of a given kind. The name SHALL be validated as above and resolved strictly inside the kind's directory. Deletion SHALL be refused with `409` while any run is queued or active, and with `404` when the file does not exist. Deleting a voice SHALL also remove its companion `.onnx.json`.

#### Scenario: delete while a run is active

- **WHEN** a run is active and a delete is requested
- **THEN** the response is `409` and the file remains

#### Scenario: delete a voice

- **WHEN** `en_US-lessac-medium.onnx` is deleted
- **THEN** both it and `en_US-lessac-medium.onnx.json` are removed

#### Scenario: unknown file

- **WHEN** the named file does not exist
- **THEN** the response is `404`

### Requirement: LoRA files are managed but not consumed

LoRA and video-model assets SHALL be listable, downloadable and deletable like other kinds. The API SHALL state, in the `lora` kind's metadata, that the pipeline does not apply LoRA yet, and, in the `video` kind's metadata, that these files are the animation models (Wan, or the AnimateDiff motion module) that a project's `defaults.animate` uses.

#### Scenario: LoRA kind metadata

- **WHEN** the asset kinds are requested
- **THEN** the `lora` kind carries a note that the pipeline does not apply LoRA yet

#### Scenario: video kind metadata

- **WHEN** the asset kinds are requested
- **THEN** the `video` kind carries a note that its files are animation models used by a project's animation settings, and no longer says that the pipeline does not generate video with them

### Requirement: Refused Hugging Face access explains how to authenticate

When a download from a `huggingface.co` host is answered with HTTP 401 or 403, the failure message SHALL say that the repository is private, gated, or does not exist, and SHALL name the steps to obtain access: accept the license on the Hugging Face page, create a read token, and start the server with `HF_TOKEN`. When a token is already configured, the message SHALL instead say that the account owning the token has no access and that the license must be accepted with that account. The message SHALL NOT contain the token. Responses from other hosts SHALL keep the plain status message.

#### Scenario: gated repository without a token

- **WHEN** a Hugging Face download is answered 401 and no `HF_TOKEN` is configured
- **THEN** the download fails with a message that mentions accepting the license, creating a read token and setting `HF_TOKEN`

#### Scenario: token without access

- **WHEN** a Hugging Face download is answered 403 and a token is configured
- **THEN** the message says the token's account has no access to the repository, and does not include the token value

#### Scenario: other hosts unchanged

- **WHEN** a non-Hugging Face URL is answered 403
- **THEN** the failure message is the plain remote status, without Hugging Face guidance







### Requirement: Catalog entries may carry a prompt hint

A catalog entry MAY include `prompt_hint`, a short plain-text string with advice about the prompt the model needs (for example required prefixes). Entries that declare none SHALL omit the field. The hint is advice for display: the server SHALL NOT alter any prompt with it. Pony Diffusion V6 XL SHALL declare a hint that names the prefixes `score_9, score_8_up, score_7_up`.

#### Scenario: Pony has a hint
- **WHEN** the catalog is listed
- **THEN** the Pony entry has a `prompt_hint` that contains `score_9, score_8_up, score_7_up`

#### Scenario: entries without a hint
- **WHEN** the Krea 2 Turbo entry is listed
- **THEN** it has no `prompt_hint` field

### Requirement: The catalog offers Juggernaut XL and Pony Diffusion as SDXL checkpoints

The catalog SHALL include `sdxl-juggernaut-xl-v9` and `sdxl-pony-diffusion-v6`, kind `model`, each a single checkpoint file from Hugging Face needing no account key, with its size and an estimated `vram_mb`. Both SHALL declare `preset` `{steps: 25}` and no `cfg_scale` or `resolution`, since those were not measured.

#### Scenario: both entries are listed
- **WHEN** the catalog is listed
- **THEN** both ids appear, kind `model`, without `requires_token`, with `preset` `{steps: 25}`

#### Scenario: installed state
- **WHEN** the checkpoint file of one of them exists in `models/`
- **THEN** its entry is marked installed



### Requirement: Catalog installs download only the missing files

When a download is started from a catalog entry, the server SHALL skip the entry's files that already exist on disk and download only the missing ones, so that bundles sharing a file (for example a VAE) install without conflict. When every file of the entry already exists, the request SHALL answer `409` saying it is already installed. Skipping SHALL NOT apply to requests that set `overwrite: true`, which replace every file.

#### Scenario: second bundle reuses a shared file

- **WHEN** a bundle is installed and a second catalog entry shares its VAE file
- **THEN** installing the second entry downloads only its other files and succeeds

#### Scenario: already installed

- **WHEN** a catalog entry is requested and all its files exist
- **THEN** the response is `409` stating it is already installed and nothing is downloaded


### Requirement: Saved keys are validated by shape and never echoed

A key SHALL be refused with `400` unless it is 8 to 512 characters long and has no whitespace or control characters, so it cannot break the file or inject a header. An unknown service SHALL be refused with `400`. Neither the key nor the path of its file SHALL appear in any response, error message or log, including the answers to refused requests. The key SHALL NOT be checked against the remote service when saved.

#### Scenario: a key with a newline
- **WHEN** the key contains a line break or a space
- **THEN** the answer is `400`, nothing is written and the existing key is unchanged

#### Scenario: too short or too long
- **WHEN** the key has 7 or 513 characters
- **THEN** the answer is `400`

#### Scenario: unknown service
- **WHEN** `PUT /api/tokens/other` is called
- **THEN** the answer is `400` and nothing is written

#### Scenario: no leaks
- **WHEN** keys are saved, refused or removed
- **THEN** no response, error or log line contains a key value or a key file path


### Requirement: Video bundles may include a high-noise expert, and Wan A14B is in the catalog

A catalog entry of kind `video` MAY have a file with `role` `high_noise_model`, besides `model`, `text_encoder` and `vae`. The catalog SHALL include "Wan 2.2 I2V A14B (Q4)" with the high-noise expert (`role` `high_noise_model`), the low-noise expert (`role` `model`), the umt5 text encoder it shares with the Wan 5B entry, and the Wan 2.1 VAE, all installed under `models/video/`; its description SHALL state the measured time, VRAM and RAM and that the files are third-party GGUF conversions. A file shared with another entry SHALL NOT be downloaded again.

#### Scenario: the A14B entry
- **WHEN** the catalog is listed
- **THEN** the A14B entry has four files with roles `high_noise_model`, `model`, `text_encoder` and `vae`, all of kind `video`

#### Scenario: shared text encoder
- **WHEN** the Wan 5B bundle is installed and the A14B bundle is then installed
- **THEN** the umt5 text encoder is not downloaded a second time

#### Scenario: installed state
- **WHEN** all four files are in `models/video/`
- **THEN** the entry reports `installed` true

### Requirement: The Models tab labels the high-noise expert

The bundle summary of a catalog entry SHALL name the `high_noise_model` role ("high-noise expert"), and a file with that role SHALL NOT be labelled a support file in the installed list.

#### Scenario: bundle line
- **WHEN** the A14B entry is shown
- **THEN** its summary lists the high-noise expert among the files

#### Scenario: not a support file
- **WHEN** the high-noise expert is installed
- **THEN** it carries no "support file" label

### Requirement: Music models are a catalog kind with bundles

The asset kinds SHALL include `music`, stored under `models/music/`. A `music` catalog entry SHALL be a bundle of files with roles `lm`, `text_encoder`, `dit` and `vae`; it SHALL be reported as installed only when all its files exist, SHALL install only missing files, and entries sharing a destination name (the text encoder, the VAE, and any language or synthesis model two bundles have in common) SHALL share the file. Each entry SHALL carry `vram_mb`, a description with its license in short and what it is for. Only models that exist in GGUF form and that the engine supports SHALL be listed, including the XL synthesis models. Besides the default bundle, entries SHALL differ from it in one file each, so that installing one after the default downloads only that file.

#### Scenario: Listing installed music models
- **WHEN** `models/music/` has some GGUF files
- **THEN** the `music` listing returns them with size and the asset kind

#### Scenario: Shared files
- **WHEN** one music bundle is installed and another that shares the text encoder and VAE is installed after it
- **THEN** the shared files are not downloaded again

#### Scenario: Partial bundle
- **WHEN** only the synthesis model of a bundle exists
- **THEN** the entry is reported as not installed

#### Scenario: A variant of the default bundle
- **WHEN** the default music bundle is installed and a variant with the XL synthesis model is installed after it
- **THEN** only the XL synthesis model is downloaded

#### Scenario: XL synthesis model is offered
- **WHEN** the catalog is read
- **THEN** a music bundle uses an XL synthesis model

### Requirement: Music assets can be deleted safely

A music file SHALL be deletable through the existing delete endpoint under the same name validation and confinement as other kinds. Deleting a file that is part of an installed bundle SHALL be allowed and SHALL make that bundle report as not installed.

#### Scenario: Delete a text encoder
- **WHEN** the text encoder of an installed bundle is deleted
- **THEN** every bundle that lists it reports as not installed

#### Scenario: Name outside the directory
- **WHEN** a delete request names a path outside `models/music/`
- **THEN** it is rejected and nothing is removed

### Requirement: Adapters are a kind installed as units

The asset kinds SHALL include `adapter`, stored under `models/music/adapters/`. An adapter SHALL be either a single `.safetensors` file or a folder holding `adapter_model.safetensors` and `adapter_config.json`. The inventory SHALL list each adapter once, by its file or folder name, with the total size of its files. A catalog entry of this kind MAY name its files as `<folder>/<file>` (one folder level, each part a bare name) and SHALL be reported as installed only when every file exists. Deleting an adapter SHALL remove its file or its whole folder, under the same name validation and confinement as other kinds. Files of other music kinds SHALL NOT appear in the adapter listing and the reverse.

#### Scenario: A folder adapter is one entry
- **WHEN** `models/music/adapters/chinese-new-year/` holds the two files
- **THEN** the listing shows one adapter `chinese-new-year` with their combined size

#### Scenario: A single file adapter
- **WHEN** `models/music/adapters/style.safetensors` exists
- **THEN** the listing shows the adapter `style.safetensors`

#### Scenario: Install from the catalog
- **WHEN** an adapter catalog entry is installed
- **THEN** its files are downloaded into its folder and the entry is reported as installed

#### Scenario: Delete a folder adapter
- **WHEN** an adapter folder is deleted
- **THEN** the whole folder is removed and nothing outside `models/music/adapters/` is touched

#### Scenario: Names stay confined
- **WHEN** a request names a path with more than one folder level or a parent reference
- **THEN** it is rejected and nothing is written


### Requirement: The catalog includes the AnimateDiff motion module as a video entry

The catalog SHALL include an entry of kind `video`, "AnimateDiff v3 (SD 1.5)", with a single file `mm_sd15_v3.safetensors` of role `motion_module`, installed under `models/video/`, a preset of 25 steps, cfg 7 and resolution 512x512, and a description stating that it needs an SD 1.5 checkpoint (installable from the Imagen tab), the measured time and VRAM, the subtle character of its motion and where its weights come from. It SHALL NOT declare a `quality`, and it SHALL NOT include a checkpoint file. It SHALL be reported installed when `models/video/mm_sd15_v3.safetensors` exists.

#### Scenario: entry shape
- **WHEN** the catalog is read
- **THEN** the AnimateDiff entry has kind `video`, one file with role `motion_module`, a preset with 25 steps, cfg 7 and 512x512 and no `quality`

#### Scenario: already downloaded
- **WHEN** `mm_sd15_v3.safetensors` is already in `models/video/`
- **THEN** the entry is reported installed and installing it again is not needed

#### Scenario: no checkpoint is downloaded
- **WHEN** the AnimateDiff entry is installed
- **THEN** only the motion module is downloaded; the SD 1.5 checkpoint already installed under Imagen is used as it is

### Requirement: Catalog entries may declare the system RAM they need

A catalog entry MAY carry `min_ram_gb`, the system RAM in GB the bundle needs to run. It SHALL be informative only (an install is never blocked by it), and the Models page SHALL show it as a warning on the entry. Entries without it SHALL be unchanged.

#### Scenario: the warning is shown
- **WHEN** an entry has `min_ram_gb: 24`
- **THEN** its card shows a warning that it needs 64 GB of system RAM

### Requirement: The catalog includes MiniMax H3 as a video bundle

The catalog SHALL include an entry of kind `video`, "MiniMax H3 Ref2VA (Q4)", with five files installed under `models/video/` (roles `model`, `text_encoder`, `vae`, `audio_vae`, `lora`), `min_ram_gb` 24, a preset of 20 steps, cfg 1 and 864x480, and a description stating that the weights are read with `--mmap`, the measured RAM, time and VRAM, and that the turbo LoRA (file role `lora`) is for drafts because the faces come out worse. It SHALL NOT declare a `quality`. It SHALL be reported installed only when the five files exist.

#### Scenario: entry shape
- **WHEN** the catalog is read
- **THEN** the MiniMax H3 entry has kind `video`, five files (roles `model`, `text_encoder`, `vae`, `audio_vae`, `lora`), `min_ram_gb` 24, a preset of 20 steps, cfg 1 and 864x480, and no `quality`

#### Scenario: partly downloaded
- **WHEN** only four of the five files are in `models/video/`
- **THEN** the entry is not reported installed

### Requirement: The catalog says how long its image models and video bundles take

Catalog entries of image models and video bundles SHALL carry `timing`: the seconds one picture or clip takes at a resolution, steps and (clips) frames on the reference machine (an RTX 5060 Ti with 16 GB), `measured` saying whether it was timed or is an estimate, and for MiniMax H3 the same with its turbo LoRA. Entries of other kinds do not.

#### Scenario: Timing of a clip
- **WHEN** the catalog is read
- **THEN** the MiniMax H3 entry says 258 s for 864x480, 56 frames and 20 steps, measured, and 96 s with 4 steps for the turbo LoRA

### Requirement: The catalog offers the first-frame MiniMax H3 bundle

The recommended catalog SHALL include "MiniMax H3 First frame (FL2VA)": the fl2va DiT, the same encoder and VAEs as the Ref2VA bundle (so they are not downloaded twice) and the 8-step first-frame LoRA, with its measured times. The editor's engine selector and Easy mode SHALL prefer it when it is installed, and the editor SHALL offer to switch between first frame and reference, swapping the DiT and the LoRA together.

#### Scenario: Both bundles
- **WHEN** the first-frame bundle is installed
- **THEN** choosing the MiniMax H3 engine writes `first_frame: true` and its DiT

### Requirement: A catalog entry can record how it fits a smaller card

A video catalog entry MAY carry `low_vram` (`vram_mb` and `resolution`): a measured way to run on a card with less memory than its preset needs. The first-frame MiniMax H3 bundle SHALL carry 11520 MB at 768x416 (measured 11.2 GB, 108 s per clip with the 8-step LoRA on an RTX 4070). Easy mode SHALL use that resolution and say so when the card cannot run the preset resolution but fits the recorded one, and SHALL leave H3 out when neither fits.

#### Scenario: A 12 GB card
- **WHEN** Easy mode plans for a 12 GB card with the first-frame bundle installed
- **THEN** the animation is MiniMax H3 at 768x416 and a note says it runs at a lower resolution

### Requirement: The catalog includes LTX 2.3 as a video bundle

The recommended catalog SHALL include an entry of kind `video`, "LTX 2.3 (distilled, Q4, vídeo con audio)", with five files installed under `models/video/` (roles `model`, `text_encoder`, `vae`, `audio_vae`, `connectors`): the distilled-1.1 Q4_K_M DiT and the video VAE, audio VAE and embeddings connectors from `unsloth/LTX-2.3-GGUF`, and the Gemma 3 12B QAT Q4 encoder from `unsloth/gemma-3-12b-it-qat-GGUF`. All of them SHALL come from repositories that need no access request. It SHALL carry `min_ram_gb` 24, `vram_mb` 11520 (measured peak 11.1 GB at the preset resolution on a 12 GB card, so it needs no `low_vram`), a preset of 8 steps, cfg 1 and 768x416, `timing` with the measured seconds, steps, frames and resolution, and a description that states the weights are read with `--mmap`, the measured RAM, time and VRAM, and that the model's audio is not used yet. It SHALL NOT declare a `quality`. It SHALL be reported installed only when the five files exist. The Gemma file SHALL NOT be shared with the Qwen3-VL encoder of MiniMax H3.

#### Scenario: entry shape
- **WHEN** the catalog is read
- **THEN** the LTX 2.3 entry has kind `video`, five files with roles `model`, `text_encoder`, `vae`, `audio_vae` and `connectors`, `min_ram_gb` 24, a preset of 8 steps, cfg 1 and 768x416, `timing` (145 s on the reference RTX 5060 Ti) and no `quality`

#### Scenario: partly downloaded
- **WHEN** only four of the five files are in `models/video/`
- **THEN** the entry is not reported installed

#### Scenario: no access request
- **WHEN** the files are downloaded without any token
- **THEN** all five downloads succeed

### Requirement: The catalog has the HeartMuLa weights

The catalog SHALL have an entry for the HeartMuLa engine (the language model, the audio codec and the three small configuration files, about 21 GB) downloadable from their Hugging Face repositories into `models/music/heartmula/`, shown with its size, its license, the memory it needs (about 10 GB of GPU) and a note that it is slower than ACE-Step. The entry SHALL appear as installed when the weights are there; the service that reads them is installed with `scripts/get-services.sh`.

#### Scenario: Download
- **WHEN** the user installs the entry
- **THEN** the files are downloaded into `models/music/heartmula/` with the usual progress

#### Scenario: Installed
- **WHEN** the weights are there
- **THEN** the entry is installed


### Requirement: The catalog has the DiffRhythm2 weights

The catalog SHALL have an entry for the DiffRhythm2 engine of its own kind, naming what it needs (the model, its configuration and its audio decoder, from the Hugging Face repository of the authors, the two MuQ models and the text encoder), which live in `models/music/diffrhythm2/` in the layout the model's libraries read. The entry SHALL be shown as installed when the folder holds the files the engine checks. The entry SHALL say its size (about 9.5 GB), its memory (8 GB of VRAM) and its limits (English lyrics, no instrumentals). The Models tab SHALL show the entry.

#### Scenario: Installed
- **WHEN** the weights are in place
- **THEN** the folder `models/music/diffrhythm2/` holds the files and the entry shows as installed

#### Scenario: Shown with its limits
- **WHEN** the catalog is shown
- **THEN** the entry says that it reads English lyrics only and makes no instrumentals

### Requirement: A catalog file can be converted while it is installed
A catalog file MAY declare `convert` with the value `int8-convrot:q8_0` or `int8-convrot:q4_K`. Its `url` is then the file as published (a ComfyUI int8 tensorwise checkpoint with convrot) and its `name` is the converted GGUF that gets installed. The conversion SHALL run once every file of the job has been downloaded and before any file is put in place: the rotation is undone into a BF16 file, `sd -M convert` quantizes it to the requested type, the result is installed under `name` and the downloaded original and the BF16 file are removed. The conversion SHALL be refused, and nothing installed, when the downloaded file is not an int8 convrot checkpoint, when the `sd` binary is not found, when the free disk space is under about 2.9 times the size of the download, or when `sd` fails; a failed or cancelled job SHALL leave no temporary file behind. The temporary files SHALL end in `.partial` so they are not listed as assets and a restart sweeps them. A file without `convert` SHALL be installed exactly as before.

#### Scenario: An int8 convrot checkpoint is installed as a GGUF
- **WHEN** a catalog entry whose model file declares `convert: "int8-convrot:q8_0"` is installed
- **THEN** the job succeeds, the asset directory holds the GGUF named by the entry and no `.partial` or intermediate file remains

#### Scenario: A file that is not int8 convrot installs nothing
- **WHEN** the downloaded file of an entry that declares `convert` is not an int8 convrot checkpoint
- **THEN** the job fails with an error saying so and nothing is installed or left over

#### Scenario: Conversion needs sd
- **WHEN** an entry that declares `convert` is installed and the `sd` binary is not found
- **THEN** the job fails with an error naming the missing binary and nothing is installed or left over

#### Scenario: Not enough disk
- **WHEN** the free disk space is under about 2.9 times the size of the downloaded file
- **THEN** the job fails before converting, with an error saying how much disk it needs

### Requirement: The conversion of int8 convrot weights is exact up to rounding
Undoing the convrot rotation SHALL reproduce the original weights: each I8 weight matrix is multiplied by its per-row `weight_scale` and, for each contiguous group of `convrot_groupsize` input features, by the normalised Kronecker power of the 4x4 regular Hadamard matrix (symmetric and orthogonal, so it is its own inverse), and written as BF16 rounded to nearest even. Group sizes that are not a power of four, or that do not divide the input dimension, SHALL be rejected. Tensors that are not I8 SHALL be copied unchanged, and the `weight_scale` and `comfy_quant` tensors SHALL be dropped.

#### Scenario: The weights come back
- **WHEN** a matrix is rotated with the Hadamard matrix, quantized to int8 with one scale per row and converted
- **THEN** the BF16 result equals the original matrix within the error of int8 and BF16 rounding (relative error under 2 %)

#### Scenario: The rotation matches the explicit matrix
- **WHEN** the in-place rotation of a group of 4, 16 or 64 values is compared with the product by the explicitly built Kronecker power of H4 over 2
- **THEN** both agree to float precision


### Requirement: Installing a model is refused when the models directory is unavailable

When a models directory is configured and it is not available, an installation (download or conversion) SHALL be refused with a message that names the directory, SHALL write nothing, and SHALL NOT fall back to a folder on the local disk.

#### Scenario: Unmounted disk
- **WHEN** a model is installed while the models disk is not mounted
- **THEN** the request fails with a message naming the models directory and no file is written
