#!/usr/bin/env bash
# scripts/get-models.sh
#
# ─────────────────────────────────────────────────────────────────────────────
#  NOT executed by this plan.  Run manually:
#      bash scripts/get-models.sh
#
#  Idempotent — safe to re-run at any time.
# ─────────────────────────────────────────────────────────────────────────────
#
# What it does
#   Downloads a stable-diffusion.cpp-compatible checkpoint into models/,
#   for use as defaults.model in scenes.yaml.
#
# Centralized constant (single source of truth — see design.md):
#   MODEL_URL   (override: MODEL_URL=https://example/other.safetensors bash scripts/get-models.sh)
#   MODEL_NAME  (override: MODEL_NAME=other.safetensors bash scripts/get-models.sh)
#
#   If the exact default checkpoint changes, update it HERE and in design.md
#   only. The default below (SD v1.5, pruned, no EMA weights dropped) is a
#   reasonable small starting point; swap it for any sd.cpp-compatible
#   .safetensors/.gguf checkpoint that fits your GPU.
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MODELS_DIR="${PROJECT_ROOT}/models"

# The default is pinned to a revision of the repository and checked against the SHA-256 that Hugging Face publishes for
# the file (the same one a local copy gives). Another MODEL_URL needs its SHA-256 in MODEL_SHA256 (ALLOW_UNVERIFIED=1
# installs it without checking, on your own risk). The repository moved from runwayml/ to stable-diffusion-v1-5/.
PINNED_REV="451f4fe16113bff5a5d2269ed5ad43b0592e9a14"
PINNED_SHA256="6ce0161689b3853acaa03779ec93eafe75a02f4ced659bee03f50797806fa2fa"
MODEL_NAME="${MODEL_NAME:-v1-5-pruned-emaonly.safetensors}"
MODEL_URL="${MODEL_URL:-https://huggingface.co/stable-diffusion-v1-5/stable-diffusion-v1-5/resolve/${PINNED_REV}/v1-5-pruned-emaonly.safetensors}"
MODEL_SHA256="${MODEL_SHA256:-}"
if [[ -z "${MODEL_SHA256}" && "${MODEL_NAME}" == "v1-5-pruned-emaonly.safetensors" && "${MODEL_URL}" == *"/${PINNED_REV}/"* ]]; then
    MODEL_SHA256="${PINNED_SHA256}"
fi

mkdir -p "${MODELS_DIR}"

MODEL_PATH="${MODELS_DIR}/${MODEL_NAME}"

if [[ -f "${MODEL_PATH}" ]]; then
    echo "[get-models] ${MODEL_NAME} already present at ${MODELS_DIR} — skipping download."
else
    echo "[get-models] Downloading ${MODEL_NAME} from ${MODEL_URL} ..."
    echo "[get-models] NOTE: if this URL 404s (checkpoint moved/gated), set MODEL_URL to a mirror and MODEL_SHA256 to its checksum."
    curl -fL "${MODEL_URL}" -o "${MODEL_PATH}.partial"
    if [[ -n "${MODEL_SHA256}" ]]; then
        got="$(sha256sum "${MODEL_PATH}.partial" | awk '{print $1}')"
        if [[ "${got}" != "${MODEL_SHA256}" ]]; then
            rm -f "${MODEL_PATH}.partial"
            echo "[get-models] ERROR: ${MODEL_NAME} does not match its checksum (got ${got}, want ${MODEL_SHA256}): removed." >&2
            exit 1
        fi
    elif [[ "${ALLOW_UNVERIFIED:-0}" == "1" ]]; then
        echo "[get-models] WARNING: no checksum for ${MODEL_NAME}: installing it unverified." >&2
    else
        rm -f "${MODEL_PATH}.partial"
        echo "[get-models] ERROR: no checksum is known for ${MODEL_NAME}: set MODEL_SHA256 or ALLOW_UNVERIFIED=1." >&2
        exit 1
    fi
    mv "${MODEL_PATH}.partial" "${MODEL_PATH}"
    echo "[get-models] ✓  Model installed at ${MODEL_PATH}"
fi

echo "[get-models] Done. Set defaults.model: ${MODEL_PATH} in scenes.yaml."
