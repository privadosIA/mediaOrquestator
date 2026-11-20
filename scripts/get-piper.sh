#!/usr/bin/env bash
# scripts/get-piper.sh
#
# ─────────────────────────────────────────────────────────────────────────────
#  NOT executed by this plan.  Run manually:
#      bash scripts/get-piper.sh
#
#  Idempotent — safe to re-run at any time.
# ─────────────────────────────────────────────────────────────────────────────
#
# What it does
#   1. Downloads the Piper TTS release binary for linux x86_64 and installs
#      it at bin/piper (on PATH when you invoke the orchestrator from the
#      project root, or symlink it onto your own PATH).
#   2. Downloads one default voice (.onnx + .onnx.json) into voices/.
#
# Centralized version/voice constants (single source of truth — see design.md):
#   PIPER_VERSION  (override:  PIPER_VERSION=2023.11.14-2 PIPER_SHA256=<sha256> bash scripts/get-piper.sh)
#   VOICE_ID       (override:  VOICE_ID=en_US-amy-medium bash scripts/get-piper.sh)
#
#   If the exact version/voice changes, update the defaults HERE and in
#   design.md only.
# ─────────────────────────────────────────────────────────────────────────────

set -euo pipefail

# ── Paths ────────────────────────────────────────────────────────────────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${PROJECT_ROOT}/bin"
VOICES_DIR="${PROJECT_ROOT}/voices"

# ── Centralized version/voice (see design.md) ────────────────────────────────
# Everything that is downloaded is pinned and checked against a SHA-256 measured on the real file: the release of
# Piper, the revision of the voices repository and the default voice. Another PIPER_VERSION, VOICE_ID or
# VOICES_REV needs its checksums in PIPER_SHA256, VOICE_ONNX_SHA256 and VOICE_JSON_SHA256 (ALLOW_UNVERIFIED=1 installs
# without checking, on your own risk).
PIPER_PINNED_VERSION="2023.11.14-2"
PIPER_PINNED_SHA256="a50cb45f355b7af1f6d758c1b360717877ba0a398cc8cbe6d2a7a3a26e225992"
VOICES_PINNED_REV="6b9cfaacb4fb2e02a9621227e3c1a48bec9710c3"
PIPER_VERSION="${PIPER_VERSION:-${PIPER_PINNED_VERSION}}"
VOICES_REV="${VOICES_REV:-${VOICES_PINNED_REV}}"
VOICE_ID="${VOICE_ID:-en_US-lessac-medium}"
PIPER_SHA256="${PIPER_SHA256:-}"
VOICE_ONNX_SHA256="${VOICE_ONNX_SHA256:-}"
VOICE_JSON_SHA256="${VOICE_JSON_SHA256:-}"
if [[ "${PIPER_VERSION}" == "${PIPER_PINNED_VERSION}" && -z "${PIPER_SHA256}" ]]; then PIPER_SHA256="${PIPER_PINNED_SHA256}"; fi
if [[ "${VOICE_ID}" == "en_US-lessac-medium" && "${VOICES_REV}" == "${VOICES_PINNED_REV}" ]]; then
    VOICE_ONNX_SHA256="${VOICE_ONNX_SHA256:-5efe09e69902187827af646e1a6e9d269dee769f9877d17b16b1b46eeaaf019f}"
    VOICE_JSON_SHA256="${VOICE_JSON_SHA256:-efe19c417bed055f2d69908248c6ba650fa135bc868b0e6abb3da181dab690a0}"
fi

# verify FILE EXPECTED NAME: removes the file and stops when its SHA-256 is not the expected one (or none is known).
verify() {
    local file="$1" want="$2" name="$3" got
    if [[ -z "${want}" ]]; then
        if [[ "${ALLOW_UNVERIFIED:-0}" == "1" ]]; then echo "[get-piper] WARNING: no checksum for ${name}: installing it unverified." >&2; return 0; fi
        rm -f "${file}"; echo "[get-piper] ERROR: no checksum is known for ${name}: set its SHA-256 (see the top of this script) or ALLOW_UNVERIFIED=1." >&2; exit 1
    fi
    got="$(sha256sum "${file}" | awk '{print $1}')"
    if [[ "${got}" != "${want}" ]]; then
        rm -f "${file}"; echo "[get-piper] ERROR: ${name} does not match its checksum (got ${got}, want ${want}): removed." >&2; exit 1
    fi
}

# Split "en_US-lessac-medium" into lang="en_US", voice="lessac", quality="medium"
IFS='-' read -r VOICE_LANG VOICE_NAME VOICE_QUALITY <<<"${VOICE_ID}"
VOICE_LANG_SHORT="${VOICE_LANG%%_*}"

PIPER_ASSET_URL="${PIPER_BASE_URL:-https://github.com/rhasspy/piper/releases/download}/${PIPER_VERSION}/piper_linux_x86_64.tar.gz"

VOICE_BASE_URL="${VOICES_BASE_URL:-https://huggingface.co/rhasspy/piper-voices/resolve}/${VOICES_REV}/${VOICE_LANG_SHORT}/${VOICE_LANG}/${VOICE_NAME}/${VOICE_QUALITY}"
VOICE_ONNX_URL="${VOICE_BASE_URL}/${VOICE_ID}.onnx"
VOICE_JSON_URL="${VOICE_BASE_URL}/${VOICE_ID}.onnx.json"

mkdir -p "${BIN_DIR}" "${VOICES_DIR}"

# ── 1. Piper binary ──────────────────────────────────────────────────────────
if [[ -x "${BIN_DIR}/piper" ]]; then
    echo "[get-piper] bin/piper already present — skipping download."
else
    echo "[get-piper] Downloading Piper (${PIPER_VERSION}) from ${PIPER_ASSET_URL} ..."
    TMP_TAR="$(mktemp)"
    curl -fL "${PIPER_ASSET_URL}" -o "${TMP_TAR}"
    verify "${TMP_TAR}" "${PIPER_SHA256}" "the Piper release"
    TMP_EXTRACT="$(mktemp -d)"
    tar -xzf "${TMP_TAR}" -C "${TMP_EXTRACT}"
    # The release tarball contains a top-level piper/ directory with the
    # binary plus its espeak-ng-data dependency; copy both into bin/.
    cp -rf "${TMP_EXTRACT}"/piper/* "${BIN_DIR}/"
    chmod +x "${BIN_DIR}/piper"
    rm -rf "${TMP_TAR}" "${TMP_EXTRACT}"
    echo "[get-piper] ✓  Binary installed at ${BIN_DIR}/piper"
fi

# ── 2. Voice model ────────────────────────────────────────────────────────────
VOICE_ONNX_PATH="${VOICES_DIR}/${VOICE_ID}.onnx"
VOICE_JSON_PATH="${VOICES_DIR}/${VOICE_ID}.onnx.json"

if [[ -f "${VOICE_ONNX_PATH}" && -f "${VOICE_JSON_PATH}" ]]; then
    echo "[get-piper] Voice '${VOICE_ID}' already present at ${VOICES_DIR} — skipping download."
else
    echo "[get-piper] Downloading voice '${VOICE_ID}' ..."
    curl -fL "${VOICE_ONNX_URL}" -o "${VOICE_ONNX_PATH}"
    verify "${VOICE_ONNX_PATH}" "${VOICE_ONNX_SHA256}" "the voice ${VOICE_ID}.onnx"
    curl -fL "${VOICE_JSON_URL}" -o "${VOICE_JSON_PATH}"
    verify "${VOICE_JSON_PATH}" "${VOICE_JSON_SHA256}" "the voice ${VOICE_ID}.onnx.json"
    echo "[get-piper] ✓  Voice installed at ${VOICE_ONNX_PATH}"
fi

echo "[get-piper] Done. Set defaults.voice: ${VOICE_ONNX_PATH} in scenes.yaml."
