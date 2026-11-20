#!/usr/bin/env bash
# scripts/get-demucs.sh
#
# Installs what "Voice separation for the timing of the words" needs: demucs.cpp (a C++ port of Demucs v4 that runs on
# the CPU, no Python, no GPU) and the htdemucs weights. Optional: without them the words are timed from the whole mix.
#
#   bash scripts/get-demucs.sh           # the weights; checks for bin/demucs
#   bash scripts/get-demucs.sh --build   # also builds the multi-threaded CLI (CPU) into bin/demucs
#
# The weights go to models/demucs/. Safe to re-run: a file that is already there with the right size and SHA-256 is not
# downloaded again, and one that does not match is deleted and the script fails naming it.
#
# demucs.cpp is built from one pinned commit (DEMUCS_CPP_REV overrides it), never from the head of a branch. The
# commit is the latest on main when this was written: 2026-11-06. It needs git, cmake, a C++17 compiler with OpenMP and
# the sources of its submodules (Eigen, libnyquist, googletest), which the script fetches. With OpenBLAS installed
# (Arch: sudo pacman -S openblas; Debian: libopenblas-openmp-dev) DEMUCS_OPENBLAS=1 builds against it.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${ROOT}/bin"
MODEL_DIR="${ROOT}/models/demucs"
BUILD=0
[[ "${1:-}" == "--build" ]] && BUILD=1

# name | url | size in bytes | sha256 (checked after the download)
# The pre-converted ggml weights are the ones the README of demucs.cpp points to (a Hugging Face dataset of the community);
# the Demucs weights are released under the MIT license of the Demucs repository (facebookresearch/demucs).
HF="https://huggingface.co"
MODELS=(
    "ggml-model-htdemucs-4s-f16.bin|${HF}/datasets/Retrobear/demucs.cpp/resolve/main/ggml-model-htdemucs-4s-f16.bin|83994361|72b17c42d308982ddb5069bc3bf48b81a5aac4cb6516e4366c0fa7cef6df0064"
)

mkdir -p "${MODEL_DIR}"
for entry in "${MODELS[@]}"; do
    IFS='|' read -r name url size sha <<<"${entry}"
    dst="${MODEL_DIR}/${name}"
    if [[ -f "${dst}" ]]; then
        if [[ "$(stat -c %s "${dst}")" == "${size}" && "$(sha256sum "${dst}" | cut -d' ' -f1)" == "${sha}" ]]; then
            echo "[get-demucs] ✓  ${name} is already there"
            continue
        fi
        rm -f "${dst}"
        echo "[get-demucs] ✗  ${name} did not match its size and SHA-256: it was deleted." >&2
        echo "[get-demucs]    Run the script again to download it." >&2
        exit 1
    fi
    echo "[get-demucs] Downloading ${name} ..."
    curl -fL "${url}" -o "${dst}.part"
    got="$(stat -c %s "${dst}.part")"
    sum="$(sha256sum "${dst}.part" | cut -d' ' -f1)"
    if [[ "${got}" != "${size}" || "${sum}" != "${sha}" ]]; then
        rm -f "${dst}.part"
        echo "[get-demucs] ✗  ${name}: expected ${size} bytes and ${sha}, got ${got} and ${sum}: nothing installed." >&2
        exit 1
    fi
    mv "${dst}.part" "${dst}"
    echo "[get-demucs] ✓  ${name}"
done

# A demucs that is there but does not start counts as missing. It has no --version: with no arguments it prints its usage.
for candidate in "${BIN_DIR}/demucs" "$(command -v demucs 2>/dev/null || true)"; do
    if [[ -n "${candidate}" && -x "${candidate}" ]] && [[ "$("${candidate}" 2>&1 || true)" == *Usage* ]]; then
        echo "[get-demucs] ✓  demucs found: ${candidate}"
        exit 0
    fi
done

if [[ "${BUILD}" == "1" ]]; then
    missing=()
    for tool in git cmake g++; do
        command -v "${tool}" >/dev/null || missing+=("${tool}")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        echo "[get-demucs] ✗  needed to build demucs: ${missing[*]} (Arch: sudo pacman -S base-devel cmake git; Debian: sudo apt-get install build-essential cmake git)" >&2
        exit 1
    fi
    SRC="${ROOT}/third_party/demucs.cpp"
    REV="${DEMUCS_CPP_REV:-4edae497a114de1ccb17ac9b4a2b8fc0144714d7}"
    REPO="${DEMUCS_CPP_REPO:-https://github.com/sevagh/demucs.cpp.git}"
    if [[ ! -d "${SRC}/.git" ]]; then git init -q "${SRC}"; git -C "${SRC}" remote add origin "${REPO}"; fi
    if [[ "$(git -C "${SRC}" rev-parse HEAD 2>/dev/null || true)" != "${REV}" ]]; then
        git -C "${SRC}" fetch -q --depth 1 origin "${REV}" || { echo "[get-demucs] ✗  cannot fetch ${REV} of demucs.cpp" >&2; exit 1; }
        git -C "${SRC}" checkout -q --detach FETCH_HEAD
    fi
    git -C "${SRC}" submodule update -q --init --recursive --depth 1 || { echo "[get-demucs] ✗  cannot fetch the submodules of demucs.cpp" >&2; exit 1; }
    echo "[get-demucs] demucs.cpp at $(git -C "${SRC}" rev-parse --short HEAD)"
    blas=OFF
    [[ "${DEMUCS_OPENBLAS:-0}" == "1" ]] && blas=ON
    # CMake 4 refuses the minimum version of the CMakeLists of demucs.cpp and its submodules; older CMake ignores the variable.
    cmake -S "${SRC}" -B "${SRC}/build" -DCMAKE_BUILD_TYPE=Release -DUSE_OPENBLAS="${blas}" -DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
        || { echo "[get-demucs] ✗  cmake failed: a C++17 compiler with OpenMP is needed (and OpenBLAS with DEMUCS_OPENBLAS=1)" >&2; exit 1; }
    cmake --build "${SRC}/build" -j --target demucs_mt.cpp.main
    mkdir -p "${BIN_DIR}"
    install -m 0755 "${SRC}/build/demucs_mt.cpp.main" "${BIN_DIR}/demucs"
    echo "[get-demucs] ✓  Built ${BIN_DIR}/demucs (CPU, multi-threaded: demucs <model> <wav> <out dir> <threads>)"
    exit 0
fi

echo "[get-demucs] ✗  demucs is not installed. Run: bash scripts/get-demucs.sh --build" >&2
exit 1
