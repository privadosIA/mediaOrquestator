#!/usr/bin/env bash
# scripts/build-acestep.sh
#
# ─────────────────────────────────────────────────────────────────────────────
#  NOT executed by any plan.  Run manually:
#      bash scripts/build-acestep.sh
#
#  Idempotent — safe to re-run at any time.
# ─────────────────────────────────────────────────────────────────────────────
#
# What it does
#   1. Clones ServeurpersoCom/acestep.cpp (recursive) into third_party/ and pins it
#      to ACESTEP_REF (the version this project was measured with).
#   2. Verifies nvcc and picks a host compiler nvcc accepts.
#   3. Builds with cmake  -DGGML_CUDA=ON -DCMAKE_BUILD_TYPE=Release
#      for the compute capabilities of the installed GPUs.
#   4. Installs bin/acestep/{ace-lm,ace-synth,ace-server} with the ggml shared libraries
#      they link, and one-line wrappers bin/ace-lm, bin/ace-synth and bin/ace-server that set
#      LD_LIBRARY_PATH, so the orchestrator finds both programs on PATH.
#
# Models are NOT downloaded here: install a music bundle from the Models
# screen (Music tab) or  POST /api/downloads {"id": "<catalog id>"}  into
# models/music/.
#
# Environment
#   NVCC_CCBIN   host compiler for nvcc (e.g. g++-13).  Rolling-release
#                distros (CachyOS/Arch with GCC 16) need an older one than the
#                system default; when unset the script tries g++-13, g++-14
#                and g++-15 if the system compiler is GCC >= 16.
#   CUDA_ARCHS   semicolon-separated architectures (e.g. "89;120"); when unset
#                it is read from nvidia-smi.
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
REPO_DIR="${PROJECT_ROOT}/third_party/acestep.cpp"
BUILD_DIR="${REPO_DIR}/build"
BIN_DIR="${PROJECT_ROOT}/bin"
LIB_DIR="${BIN_DIR}/acestep"
REPO_URL="https://github.com/ServeurpersoCom/acestep.cpp.git"

# ── Centralized build flags ──────────────────────────────────────────────────
ACE_CMAKE_FLAGS=(
    -DGGML_CUDA=ON
    -DCMAKE_BUILD_TYPE=Release
)
ACE_PROGRAMS=(ace-lm ace-synth ace-server)

# ── 1. Clone / pin (idempotent) ──────────────────────────────────────────────
# The engine is built from ACESTEP_REF, not from "whatever upstream has now":
# the request fields, model names and output naming this project relies on were
# measured on that version (upstream's own docs have been wrong about them
# before). Raising it is a deliberate change: set the new commit here, rebuild,
# and run the smoke test (go test ./internal/music/music).
ACESTEP_REF="${ACESTEP_REF:-d881ad2c080bac65cc73cb3bbcc831f79462b98b}"

if [[ -d "${REPO_DIR}/.git" ]]; then
    echo "[build-acestep] Repo already present at ${REPO_DIR} — fetching."
    git -C "${REPO_DIR}" fetch --tags origin
else
    echo "[build-acestep] Cloning ${REPO_URL} (recursive) …"
    mkdir -p "$(dirname "${REPO_DIR}")"
    git clone --recurse-submodules "${REPO_URL}" "${REPO_DIR}"
fi
echo "[build-acestep] Pinning to ${ACESTEP_REF}"
git -C "${REPO_DIR}" checkout --detach "${ACESTEP_REF}"
git -C "${REPO_DIR}" submodule update --init --recursive

# ── 2. nvcc and host compiler ────────────────────────────────────────────────
if ! command -v nvcc &>/dev/null; then
    echo "[build-acestep] ERROR: 'nvcc' not found in PATH." >&2
    echo "[build-acestep]        Install the CUDA Toolkit and put nvcc on PATH." >&2
    exit 1
fi
echo "[build-acestep] $(nvcc --version | tail -n1)"

CCBIN="${NVCC_CCBIN:-}"
if [[ -z "${CCBIN}" ]]; then
    GCC_MAJOR="$(gcc -dumpversion 2>/dev/null | cut -d. -f1 || echo 0)"
    if (( GCC_MAJOR >= 16 )); then
        for candidate in g++-13 g++-14 g++-15; do
            if command -v "${candidate}" &>/dev/null; then CCBIN="${candidate}"; break; fi
        done
        if [[ -z "${CCBIN}" ]]; then
            echo "[build-acestep] ERROR: GCC ${GCC_MAJOR} is too new for nvcc and no g++-13/14/15 was found." >&2
            echo "[build-acestep]        Install an older GCC and re-run with NVCC_CCBIN=g++-13." >&2
            exit 1
        fi
    fi
fi
if [[ -n "${CCBIN}" ]]; then
    echo "[build-acestep] nvcc host compiler: ${CCBIN}"
    ACE_CMAKE_FLAGS+=("-DCMAKE_CUDA_HOST_COMPILER=${CCBIN}")
fi

# ── 3. Architectures, configure & build ──────────────────────────────────────
ARCHS="${CUDA_ARCHS:-}"
if [[ -z "${ARCHS}" ]] && command -v nvidia-smi &>/dev/null; then
    ARCHS="$(nvidia-smi --query-gpu=compute_cap --format=csv,noheader | tr -d '. ' | sort -un | paste -sd';')"
fi
if [[ -n "${ARCHS}" ]]; then
    echo "[build-acestep] CUDA architectures: ${ARCHS}"
    ACE_CMAKE_FLAGS+=("-DCMAKE_CUDA_ARCHITECTURES=${ARCHS}")
fi

mkdir -p "${BUILD_DIR}"
echo "[build-acestep] cmake configure: ${ACE_CMAKE_FLAGS[*]}"
cmake -S "${REPO_DIR}" -B "${BUILD_DIR}" "${ACE_CMAKE_FLAGS[@]}"

JOBS="$(nproc 2>/dev/null || echo 4)"
echo "[build-acestep] cmake build (-j${JOBS}) …"
cmake --build "${BUILD_DIR}" --config Release -j "${JOBS}"

# ── 4. Install programs, libraries and wrappers ──────────────────────────────
mkdir -p "${LIB_DIR}"
for prog in "${ACE_PROGRAMS[@]}"; do
    if [[ ! -x "${BUILD_DIR}/${prog}" ]]; then
        echo "[build-acestep] ERROR: ${BUILD_DIR}/${prog} was not built." >&2
        exit 1
    fi
    cp -f "${BUILD_DIR}/${prog}" "${LIB_DIR}/${prog}"
done
# The programs link ggml dynamically; copy the versioned libraries beside them.
cp -fP "${BUILD_DIR}"/libggml*.so* "${LIB_DIR}/"

for prog in "${ACE_PROGRAMS[@]}"; do
    cat > "${BIN_DIR}/${prog}" <<WRAP
#!/usr/bin/env bash
# Generated by scripts/build-acestep.sh
HERE="\$(cd "\$(dirname "\${BASH_SOURCE[0]}")" && pwd)/acestep"
export LD_LIBRARY_PATH="\${HERE}\${LD_LIBRARY_PATH:+:\${LD_LIBRARY_PATH}}"
exec "\${HERE}/${prog}" "\$@"
WRAP
    chmod +x "${BIN_DIR}/${prog}"
done

echo "[build-acestep] ✓  Installed ${BIN_DIR}/ace-lm, ace-synth and ace-server (programs in ${LIB_DIR})"
echo "[build-acestep]    Next: install a music bundle into models/music/ (Models screen, Music tab)."
