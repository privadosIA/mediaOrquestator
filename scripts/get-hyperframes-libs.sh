#!/usr/bin/env bash
# scripts/get-hyperframes-libs.sh
#
# Downloads, once, the libraries the HyperFrames blocks load from a CDN (gsap, three and its loaders, d3, topojson, lottie,
# draco, map data, Inter), so that drawing a block never touches the network. The list, with the SHA-256 of every file, is
# internal/music/hyperframes/libs.txt; each file goes to third_party/hyperframes-libs/<host>/<path>, which is not committed
# (see THIRD_PARTY.md for the licences). Safe to re-run: a file already there with the right hash is not downloaded again,
# and a file that does not match its hash is never installed.
#
#   bash scripts/get-hyperframes-libs.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
LIST="${ROOT}/internal/music/hyperframes/libs.txt"
DEST="${ROOT}/third_party/hyperframes-libs"

sha() { sha256sum "$1" | cut -d' ' -f1; }

have=0; got=0; bad=0
while read -r hash url; do
    [[ -z "${hash}" || "${hash}" == \#* ]] && continue
    rel="${url#https://}"
    file="${DEST}/${rel}"
    if [[ -f "${file}" && "$(sha "${file}")" == "${hash}" ]]; then
        have=$((have + 1)); continue
    fi
    tmp="$(mktemp)"
    if ! curl -fsSL "${url}" -o "${tmp}"; then
        echo "[get-hyperframes-libs] ✗  cannot download ${url}" >&2; rm -f "${tmp}"; bad=$((bad + 1)); continue
    fi
    if [[ "$(sha "${tmp}")" != "${hash}" ]]; then
        echo "[get-hyperframes-libs] ✗  ${url}: sha256 does not match the pinned one: not installed" >&2
        rm -f "${tmp}"; bad=$((bad + 1)); continue
    fi
    mkdir -p "$(dirname "${file}")"
    install -m 0644 "${tmp}" "${file}"; rm -f "${tmp}"
    got=$((got + 1))
done < "${LIST}"

echo "[get-hyperframes-libs] ${have} already there, ${got} downloaded, ${bad} failed (in ${DEST})"
[[ "${bad}" -eq 0 ]]
