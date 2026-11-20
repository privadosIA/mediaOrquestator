#!/usr/bin/env bash
# scripts/get-gsap.sh
#
# Installs the GSAP core (gsap.min.js) that parts and scenes use when they animate with a GSAP timeline
# (window.__timelines, class="clip" data-start data-duration). Optional: without it only the parts and scenes that
# use GSAP are unavailable; everything that moves with window.__partSeek works as always.
#
#   bash scripts/get-gsap.sh
#
# The file comes from the npm registry tarball of ONE pinned version and its SHA-256 is checked against the one written
# here; it goes to third_party/gsap/gsap.min.js, which is not committed (see THIRD_PARTY.md for the licence). Safe to
# re-run: a file that is there with the right hash is not downloaded again.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEST_DIR="${ROOT}/third_party/gsap"
DEST="${DEST_DIR}/gsap.min.js"

VERSION="3.13.0"
SHA256="96c01b81f44a3290e2b4532f55e2c9534b2adc43273a19f3756b2cb41f0fd0b6"
URL="https://registry.npmjs.org/gsap/-/gsap-${VERSION}.tgz"

sha() { sha256sum "$1" | cut -d' ' -f1; }

if [[ -f "${DEST}" ]]; then
    if [[ "$(sha "${DEST}")" == "${SHA256}" ]]; then
        echo "[get-gsap] ✓  gsap ${VERSION} is already there: ${DEST}"
        exit 0
    fi
    echo "[get-gsap] ✗  ${DEST} does not match the pinned hash of gsap ${VERSION}: removed" >&2
    rm -f "${DEST}"
fi

work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT
echo "[get-gsap] Downloading gsap ${VERSION} ..."
curl -fsSL "${URL}" -o "${work}/gsap.tgz" || { echo "[get-gsap] ✗  cannot download ${URL}" >&2; exit 1; }
tar -xzf "${work}/gsap.tgz" -C "${work}" package/dist/gsap.min.js || { echo "[get-gsap] ✗  the tarball has no dist/gsap.min.js" >&2; exit 1; }
got="$(sha "${work}/package/dist/gsap.min.js")"
if [[ "${got}" != "${SHA256}" ]]; then
    echo "[get-gsap] ✗  gsap.min.js of ${VERSION}: expected sha256 ${SHA256}, got ${got}: nothing installed." >&2
    exit 1
fi
mkdir -p "${DEST_DIR}"
install -m 0644 "${work}/package/dist/gsap.min.js" "${DEST}"
echo "[get-gsap] ✓  gsap ${VERSION} installed: ${DEST}"
echo "[get-gsap]    check: sha256sum ${DEST}  (${SHA256})"
