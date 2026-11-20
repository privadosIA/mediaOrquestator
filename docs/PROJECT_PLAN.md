> **STATUS (2026-10-08): DONE.** The CLI described here is implemented, tested and spec'd
> (change `add-script-to-video-pipeline`, archived in `openspec/changes/archive/`). This document
> is kept as the original planning prompt; the current reference is `README.md` + `openspec/specs/`.

# TASK: Build Headless Media Orchestrator CLI in Go

You are an expert systems and Go engineer. Your objective is to build a headless, reproducible multimedia pipeline orchestrator in Go that integrates C/C++ native binaries (Piper TTS, stable-diffusion.cpp, and FFmpeg) without using any Python runtime or GUI dependencies.

---

## 1. System Requirements & Architecture Constraints

- **Language:** Go (>= 1.22), idiomatic standard library (`os/exec`, `context`, `io.Reader/Writer`).
- **External Binaries:**
  - `./bin/piper` (C++ fast TTS via ONNX runtime)
  - `./bin/sd` (C/C++ CUDA inference via stable-diffusion.cpp)
  - `ffmpeg` & `ffprobe` (system packages)
- **Zero Python Guarantee:** Do not use `pip`, `venv`, `python` wrappers, or REST server frameworks.
- **Resource Discipline:** Subprocess-based execution. Every binary process must terminate after completing its stage, ensuring immediate VRAM and RAM reclamation.

---

## 2. Directory Layout Specification

Ensure the workspace strictly reflects this directory tree:

```text
media-orchestrator/
├── bin/
│   ├── sd                  # Native binary built with SD_CUBLAS=ON
│   └── piper               # Native Piper C++ executable
├── models/
│   └── v1-5-pruned-emaonly.safetensors
├── voices/
│   ├── es_ES-davefx-medium.onnx
│   └── es_ES-davefx-medium.onnx.json
├── tmp/                    # Intermediate ephemeral assets (.wav, .png, .mp4)
├── scenes.yaml             # Input declarative configuration
├── go.mod
├── go.sum
└── main.go                 # Orchestrator core
```

---

## 3. Step-by-Step Execution Plan

### Step 1: Toolchain & Workspace Initialization

1. Initialize Go module: `go mod init media-orchestrator`.
2. Add YAML parsing dependency: `go get gopkg.in/yaml.v3`.
3. Create directories: `mkdir -p bin models voices tmp`.
4. Verify system tools availability: check `ffmpeg`, `ffprobe`, `cmake`, `gcc`, and CUDA compiler (`nvcc`).

### Step 2: Native Binaries & Assets Retrieval

1. **Compile stable-diffusion.cpp:**
   - Clone `https://github.com/leejet/stable-diffusion.cpp.git` into `/tmp/sd-build`.
   - Build with CUDA acceleration: `cmake -B build -DSD_CUBLAS=ON -DCMAKE_BUILD_TYPE=Release && cmake --build build --config Release -j$(nproc)`.
   - Move `build/bin/sd` to `./bin/sd` and grant executable permissions (`chmod +x`).
2. **Setup Piper TTS:**
   - Fetch the latest precompiled release tarball for Linux x86_64 from `rhasspy/piper`.
   - Extract and place the `piper` executable in `./bin/piper`.
3. **Download Model Assets:**
   - Place an SD model in `./models/` (e.g., Stable Diffusion 1.5 `.safetensors` or quantized GGUF).
   - Download the Spanish voice model files (`.onnx` and `.onnx.json`) into `./voices/`.

### Step 3: Orchestrator Core Implementation (`main.go`)

Implement the single-file orchestrator adhering to the following module contracts:

1. **Config Parser:**
   - Define Go structs matching `scenes.yaml` schema (`Project`, `OutputFile`, `Defaults`, and `[]Scene`).
   - Validate file paths prior to running any inference steps.
2. **Audio Synthesis Stage (`runTTS`):**
   - Execute `./bin/piper` with `--model` and `--output_file`.
   - Stream `scene.TTSText` directly into the subprocess's `stdin` via `io.WriteString` to avoid temp text files.
3. **Temporal Telemetry Stage (`getAudioDuration`):**
   - Run `ffprobe -v error -show_entries format=duration -of default=noprint_wrappers=1:nokey=1 <wav_path>`.
   - Parse stdout to `float64` to determine exact audio length in seconds.
4. **Visual Diffusion Stage (`runDiffusion`):**
   - Execute `./bin/sd` with `-m <model>`, `-p <prompt>`, `-o <png_path>`, `--steps`, `-W`, `-H`, `--sampling-method`, and `--seed`.
   - Wait for process exit to ensure VRAM release before moving to assembly.
5. **Intermediate Clipping Stage (`buildSceneClip`):**
   - Run `ffmpeg -y -loop 1 -i <png> -i <wav> -c:v libx264 -tune stillimage -c:a aac -b:a 192k -pix_fmt yuv420p -shortest <scene_mp4>`.
6. **Final Assembly Stage (`concatenateVideos`):**
   - Generate `tmp/concat_list.txt` referencing all intermediate MP4s.
   - Run `ffmpeg -y -f concat -safe 0 -i <concat_list> -c copy <output_file>`.

### Step 4: Verification & Smoke Test

1. Create a minimal `scenes.yaml` with 2 scenes.
2. Build the Go binary: `go build -o orchestrator main.go`.
3. Run `./orchestrator scenes.yaml`.
4. Validate that `output.mp4` contains properly synchronized audio and video frames.
5. Verify that `tmp/` intermediate files can be safely deleted or kept for caching.

---

## 4. Input Configuration Schema (`scenes.yaml`)

```yaml
project: "cyberpunk_chronicles"
output_file: "output.mp4"

defaults:
  model_diffusion: "models/v1-5-pruned-emaonly.safetensors"
  voice_onnx: "voices/es_ES-davefx-medium.onnx"
  resolution:
    width: 1024
    height: 576
  sampling_method: "euler_a"
  steps: 20

scenes:
  - id: 1
    prompt: "cyberpunk server rack, glowing blue optical cables, dark industrial room, cinematic"
    negative_prompt: "blurry, low quality, oversaturated"
    seed: 849204
    tts_text: "La unidad de procesamiento central ha detectado una anomalía en el sector de memoria."
    speed: 1.0

  - id: 2
    prompt: "futuristic terminal screen displaying network topology, red warning alerts, high detail"
    negative_prompt: "cartoon, drawing, lowres"
    seed: 391023
    tts_text: "Iniciando protocolo de aislamiento y reconfiguración de tablas de enrutamiento."
    speed: 0.95
```