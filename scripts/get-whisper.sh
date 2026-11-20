#!/usr/bin/env bash
# scripts/get-whisper.sh
#
# Installs what "Take it from a video or audio" needs to read the lyrics of a song:
# whisper.cpp's whisper-cli and a Whisper model. Optional: without them only the
# lyrics extraction is missing.
#
#   bash scripts/get-whisper.sh           # the model; checks for whisper-cli
#   bash scripts/get-whisper.sh --build   # also builds whisper-cli (CPU) into bin/
#
# whisper-cli is looked for in bin/ first and then in the PATH (Arch has it in
# the package whisper.cpp). The model goes to models/whisper/. Safe to re-run:
# a file that is already there with the right size is not downloaded again.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${ROOT}/bin"
MODEL_DIR="${ROOT}/models/whisper"
BUILD=0
[[ "${1:-}" == "--build" ]] && BUILD=1

HF="https://huggingface.co"
# name | url | size in bytes (checked after the download)
MODELS=(
    "ggml-large-v3-turbo-q5_0.bin|${HF}/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin|574041195"
)

mkdir -p "${MODEL_DIR}"
for entry in "${MODELS[@]}"; do
    IFS='|' read -r name url size <<<"${entry}"
    dst="${MODEL_DIR}/${name}"
    if [[ -f "${dst}" && "$(stat -c %s "${dst}")" == "${size}" ]]; then
        echo "[get-whisper] ✓  ${name} is already there"
        continue
    fi
    echo "[get-whisper] Downloading ${name} ..."
    curl -fL "${url}" -o "${dst}.part"
    got="$(stat -c %s "${dst}.part")"
    if [[ "${got}" != "${size}" ]]; then
        rm -f "${dst}.part"
        echo "[get-whisper] ✗  ${name}: expected ${size} bytes, got ${got}: nothing installed." >&2
        exit 1
    fi
    mv "${dst}.part" "${dst}"
    echo "[get-whisper] ✓  ${name}"
done

# A whisper-cli that is there but does not start (a package built against a
# library that was later updated, e.g. libopenvino) counts as missing.
for candidate in "${BIN_DIR}/whisper-cli" "$(command -v whisper-cli 2>/dev/null || true)"; do
    if [[ -n "${candidate}" && -x "${candidate}" ]] && "${candidate}" --help >/dev/null 2>&1; then
        echo "[get-whisper] ✓  whisper-cli found: ${candidate}"
        exit 0
    fi
done

if [[ "${BUILD}" == "1" ]]; then
    for tool in git cmake; do
        command -v "${tool}" >/dev/null || { echo "[get-whisper] ✗  ${tool} is needed to build whisper-cli" >&2; exit 1; }
    done
    SRC="${ROOT}/third_party/whisper.cpp"
    # whisper.cpp is built from one pinned commit (WHISPER_CPP_REV overrides it), never from the head of a branch.
    REV="${WHISPER_CPP_REV:-231638a0717b7822784297e5980965ffcec05810}"
    REPO="${WHISPER_CPP_REPO:-https://github.com/ggml-org/whisper.cpp.git}"
    if [[ ! -d "${SRC}/.git" ]]; then git init -q "${SRC}"; git -C "${SRC}" remote add origin "${REPO}"; fi
    if [[ "$(git -C "${SRC}" rev-parse HEAD 2>/dev/null || true)" != "${REV}" ]]; then
        git -C "${SRC}" fetch -q --depth 1 origin "${REV}" || { echo "[get-whisper] ✗  cannot fetch ${REV} of whisper.cpp" >&2; exit 1; }
        git -C "${SRC}" checkout -q --detach FETCH_HEAD
    fi
    echo "[get-whisper] whisper.cpp at $(git -C "${SRC}" rev-parse --short HEAD)"
    cmake -S "${SRC}" -B "${SRC}/build" -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
    cmake --build "${SRC}/build" -j --target whisper-cli
    mkdir -p "${BIN_DIR}"
    install -m 0755 "${SRC}/build/bin/whisper-cli" "${BIN_DIR}/whisper-cli"
    echo "[get-whisper] ✓  Built ${BIN_DIR}/whisper-cli (CPU)"
    exit 0
fi

echo "[get-whisper] ✗  whisper-cli is not installed. Install the package (Arch: sudo pacman -S whisper.cpp)" >&2
echo "[get-whisper]    or run: bash scripts/get-whisper.sh --build" >&2
exit 1
