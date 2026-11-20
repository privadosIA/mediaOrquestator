#!/usr/bin/env bash
# scripts/get-ytdlp.sh
#
# Installs yt-dlp (the program that takes the audio of a YouTube video, used
# to add a source from a URL) at bin/yt-dlp. It is the official standalone
# binary: no Python needed. The download is checked against the SHA-256 sums
# published with the release.
#
#   bash scripts/get-ytdlp.sh        # install, or update when already there
#
# Safe to re-run: YouTube changes often and yt-dlp is updated to follow it, so
# running this again fetches the latest release.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)/bin"
BASE="https://github.com/yt-dlp/yt-dlp/releases/latest/download"
ASSET="yt-dlp_linux"

mkdir -p "${BIN_DIR}"
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

echo "[get-ytdlp] Downloading ${ASSET} ..."
curl -fL "${BASE}/${ASSET}" -o "${TMP}/${ASSET}"
curl -fL "${BASE}/SHA2-256SUMS" -o "${TMP}/SHA2-256SUMS"

want="$(awk -v a="${ASSET}" '$2 == a || $2 == "*"a {print $1}' "${TMP}/SHA2-256SUMS")"
got="$(sha256sum "${TMP}/${ASSET}" | awk '{print $1}')"
if [[ -z "${want}" || "${want}" != "${got}" ]]; then
    echo "[get-ytdlp] ✗  checksum mismatch (expected '${want}', got '${got}'): nothing installed." >&2
    exit 1
fi

install -m 0755 "${TMP}/${ASSET}" "${BIN_DIR}/yt-dlp"
echo "[get-ytdlp] ✓  Installed ${BIN_DIR}/yt-dlp ($("${BIN_DIR}/yt-dlp" --version))"
