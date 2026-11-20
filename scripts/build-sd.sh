#!/usr/bin/env bash
# scripts/build-sd.sh
#
# ─────────────────────────────────────────────────────────────────────────────
#  NOT executed by this plan.  Run manually:
#      bash scripts/build-sd.sh
#
#  Idempotent — safe to re-run at any time.
# ─────────────────────────────────────────────────────────────────────────────
#
# What it does
#   1. Clones leejet/stable-diffusion.cpp (recursive) into third_party/ and pins it
#      to SD_REF (the version this project was measured with).
#   2. Verifies nvcc >= 12.8 (sm_120 / Blackwell requires it).
#   3. Builds with cmake  -DSD_CUDA=ON -DCMAKE_BUILD_TYPE=Release
#      (explicitly NOT the deprecated -DSD_CUBLAS flag).
#   4. Copies the resulting binary to bin/sd.
#
# Centralized flag constants (single source of truth — see design.md):
#   SD_CMAKE_FLAGS  = ( -DSD_CUDA=ON  -DCMAKE_BUILD_TYPE=Release )
#   SD_BIN_TARGET   = bin/sd
#
#   If the exact flags change, update them HERE and in design.md only.
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="${PROJECT_ROOT}/third_party/stable-diffusion.cpp"
BIN_DIR="${PROJECT_ROOT}/bin"
REPO_URL="https://github.com/leejet/stable-diffusion.cpp.git"

# ── Centralized build flags (see design.md) ──────────────────────────────────
#   -DSD_CUDA=ON            → enable CUDA backend (replaces old -DSD_CUBLAS)
#   -DCMAKE_BUILD_TYPE=Release
#   -DSD_CUBLAS             → DEPRECATED, intentionally NOT passed
# SD_CUDA=OFF builds for the CPU only (no CUDA, no nvcc check): renders are then
# very slow. Used by scripts/install.sh --cpu-sd on machines without an NVIDIA GPU.
SD_CUDA="${SD_CUDA:-ON}"
SD_CMAKE_FLAGS=(
    -DSD_CUDA="${SD_CUDA}"
    -DCMAKE_BUILD_TYPE=Release
)

# ── 1. Clone / pin (idempotent) ──────────────────────────────────────────────
# sd is built from SD_REF (master-949, the version every benchmark in benchmarks/ was
# measured with), not from "whatever upstream has now": master-1010 (012c7f8) makes LTX 2.3
# write an all-white clip (the raw video that sd saves is already 255 in every frame, the
# renders and the other animation engines are fine). Raising it is a deliberate change: set
# the new commit here, rebuild, and run `python3 scripts/benchmark.py --keep --only ltx`,
# then look at the clip (the benchmark only checks that the run ends, not what it drew).
SD_REF="${SD_REF:-7867f6da4af741d4d9d5fbd6c7f1197dfb8c6e39}"

if [[ -d "${REPO_DIR}/.git" ]]; then
    echo "[build-sd] Repo already present at ${REPO_DIR} — fetching."
    git -C "${REPO_DIR}" fetch --tags origin
else
    echo "[build-sd] Cloning ${REPO_URL} (recursive) …"
    mkdir -p "$(dirname "${REPO_DIR}")"
    git clone --recursive "${REPO_URL}" "${REPO_DIR}"
fi
echo "[build-sd] Pinning to ${SD_REF}"
git -C "${REPO_DIR}" checkout --detach "${SD_REF}"
git -C "${REPO_DIR}" submodule update --init --recursive

# ── 2. Verify nvcc >= 12.8 (CUDA builds only) ────────────────────────────────
if [[ "${SD_CUDA}" == "OFF" ]]; then
    echo "[build-sd] SD_CUDA=OFF: CPU-only build, the nvcc check is skipped (renders will be very slow)."
else
    if ! command -v nvcc &>/dev/null; then
        echo "[build-sd] ERROR: 'nvcc' not found in PATH." >&2
        echo "[build-sd]        Install CUDA Toolkit >= 12.8 and ensure nvcc is on PATH." >&2
        exit 1
    fi

    # Extract "X.Y" from e.g. "Cuda compilation tools, release 12.8, V12.8.61"
    NVCC_VERSION="$(nvcc --version 2>/dev/null | grep -oP 'release \K[0-9]+\.[0-9]+')"
    if [[ -z "${NVCC_VERSION}" ]]; then
        echo "[build-sd] ERROR: Could not parse nvcc version from: $(nvcc --version 2>&1)" >&2
        exit 1
    fi

    NVCC_MAJOR="${NVCC_VERSION%%.*}"
    NVCC_MINOR="${NVCC_VERSION#*.}"

    if (( NVCC_MAJOR < 12 )) || { (( NVCC_MAJOR == 12 )) && (( NVCC_MINOR < 8 )); }; then
        echo "[build-sd] ERROR: nvcc ${NVCC_VERSION} is too old." >&2
        echo "[build-sd]        sm_120 (NVIDIA Blackwell) requires CUDA Toolkit >= 12.8." >&2
        echo "[build-sd]        Upgrade your CUDA installation and re-run this script." >&2
        exit 1
    fi

    echo "[build-sd] nvcc ${NVCC_VERSION} ✓  (>= 12.8 required for sm_120 / Blackwell)"
fi

# ── 3. Configure & build ─────────────────────────────────────────────────────
BUILD_DIR="${REPO_DIR}/build"
mkdir -p "${BUILD_DIR}"

echo "[build-sd] cmake configure: ${SD_CMAKE_FLAGS[*]}"
cmake -S "${REPO_DIR}" -B "${BUILD_DIR}" "${SD_CMAKE_FLAGS[@]}"

JOBS="$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 4)"
echo "[build-sd] cmake build (-j${JOBS}) …"
cmake --build "${BUILD_DIR}" --config Release -j "${JOBS}"

# ── 4. Install binary ────────────────────────────────────────────────────────
mkdir -p "${BIN_DIR}"
# Upstream renamed the CLI target from `sd` to `sd-cli` (under build/bin/);
# accept either layout.
SD_BUILT=""
for candidate in "${BUILD_DIR}/bin/sd-cli" "${BUILD_DIR}/bin/sd" "${BUILD_DIR}/sd"; do
    if [[ -x "${candidate}" ]]; then SD_BUILT="${candidate}"; break; fi
done
if [[ -z "${SD_BUILT}" ]]; then
    echo "[build-sd] ERROR: built binary not found under ${BUILD_DIR} (looked for bin/sd-cli, bin/sd, sd)." >&2
    exit 1
fi
cp -f "${SD_BUILT}" "${BIN_DIR}/sd"
chmod +x "${BIN_DIR}/sd"

echo "[build-sd] ✓  Binary installed at ${BIN_DIR}/sd"