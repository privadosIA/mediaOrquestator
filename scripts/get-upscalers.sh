#!/usr/bin/env bash
# scripts/get-upscalers.sh
#
# Installs the two programs the video-edit tools use for the better engines of "scale" and "frame rate":
#
#   realesrgan  Real-ESRGAN (ncnn, Vulkan), model realesr-animevideov3: enlarges a video with a network -> bin/realesrgan/
#   rife        RIFE (ncnn, Vulkan): makes the frames in between with a network                 -> bin/rife/
#
# Both are the authors' own builds for Linux, run on any graphics card with Vulkan (NVIDIA, AMD or Intel) and need no
# CUDA and no Python. Without them the tools still work, with the lanczos filter and minterpolate, and say so.
#
#   bash scripts/get-upscalers.sh                    # both
#   bash scripts/get-upscalers.sh --only realesrgan  # one of them
#   bash scripts/get-upscalers.sh --force            # download again even when it is there
#
# The downloads are checked against the SHA-256 recorded below (they are the files of fixed releases, not "latest").
# RIFE's download is 430 MB because it carries every version of the network; only the program and the one network the
# tool uses are kept (about 20 MB). Safe to re-run: what is there is left alone.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
BIN_DIR="${ROOT}/bin"
ONLY=""
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --only) ONLY="${2:?--only needs realesrgan or rife}"; shift 2 ;;
        --force) FORCE=1; shift ;;
        -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
        *) echo "unknown option '$1' (try --help)" >&2; exit 2 ;;
    esac
done
[[ -z "${ONLY}" || "${ONLY}" == realesrgan || "${ONLY}" == rife ]] || { echo "[get-upscalers] --only is realesrgan or rife, not '${ONLY}'" >&2; exit 2; }

command -v curl >/dev/null || { echo "[get-upscalers] curl is needed" >&2; exit 1; }
command -v unzip >/dev/null || { echo "[get-upscalers] unzip is needed" >&2; exit 1; }

REALESRGAN_URL="https://github.com/xinntao/Real-ESRGAN/releases/download/v0.2.5.0/realesrgan-ncnn-vulkan-20220424-ubuntu.zip"
REALESRGAN_SHA="e5aa6eb131234b87c0c51f82b89390f5e3e642b7b70f2b9bbe95b6a285a40c96"
RIFE_URL="https://github.com/nihui/rife-ncnn-vulkan/releases/download/20221029/rife-ncnn-vulkan-20221029-ubuntu.zip"
RIFE_SHA="1e2c7ee7fa7daa326542d50622f0afedc80cf6f1858bda411d16385ffa5cdf68"
RIFE_MODEL="rife-v4.6"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

fetch() { # url sha out
    echo "[get-upscalers] Downloading $(basename "$1") ..."
    curl -fL --retry 3 -o "$3" "$1"
    local got
    got="$(sha256sum "$3" | awk '{print $1}')"
    if [[ "${got}" != "$2" ]]; then
        echo "[get-upscalers] ✗  checksum mismatch for $(basename "$1") (expected $2, got ${got}): nothing installed." >&2
        exit 1
    fi
}

install_realesrgan() {
    local dir="${BIN_DIR}/realesrgan"
    if [[ "${FORCE}" == 0 && -x "${dir}/realesrgan-ncnn-vulkan" && -f "${dir}/models/realesr-animevideov3-x2.bin" ]]; then
        echo "[get-upscalers] ✓  realesrgan is already at ${dir}"; return
    fi
    fetch "${REALESRGAN_URL}" "${REALESRGAN_SHA}" "${TMP}/realesrgan.zip"
    rm -rf "${TMP}/realesrgan" "${dir}.new"
    mkdir -p "${TMP}/realesrgan" "${dir}.new"
    # The program and one family of models, realesr-animevideov3 (x2, x3, x4; 1.2 MB each): the one made for video,
    # and the best of the three compared on frames of generated video (the photo model, realesrgan-x4plus, barely
    # beats lanczos, and the anime one paints the skin). The zip also carries those two, two demo pictures and a demo video.
    unzip -q "${TMP}/realesrgan.zip" 'realesrgan-ncnn-vulkan' 'models/realesr-animevideov3-*' -d "${TMP}/realesrgan"
    cp -r "${TMP}/realesrgan/realesrgan-ncnn-vulkan" "${TMP}/realesrgan/models" "${dir}.new/"
    chmod 0755 "${dir}.new/realesrgan-ncnn-vulkan"
    rm -rf "${dir}"
    mv "${dir}.new" "${dir}"
    echo "[get-upscalers] ✓  realesrgan -> ${dir} ($(du -sh "${dir}" | cut -f1))"
}

install_rife() {
    local dir="${BIN_DIR}/rife"
    if [[ "${FORCE}" == 0 && -x "${dir}/rife-ncnn-vulkan" && -f "${dir}/${RIFE_MODEL}/flownet.bin" ]]; then
        echo "[get-upscalers] ✓  rife is already at ${dir}"; return
    fi
    fetch "${RIFE_URL}" "${RIFE_SHA}" "${TMP}/rife.zip"
    rm -rf "${TMP}/rife" "${dir}.new"
    mkdir -p "${TMP}/rife" "${dir}.new"
    local top="rife-ncnn-vulkan-20221029-ubuntu"
    unzip -q "${TMP}/rife.zip" "${top}/rife-ncnn-vulkan" "${top}/LICENSE" "${top}/${RIFE_MODEL}/*" -d "${TMP}/rife"
    cp -r "${TMP}/rife/${top}/rife-ncnn-vulkan" "${TMP}/rife/${top}/LICENSE" "${TMP}/rife/${top}/${RIFE_MODEL}" "${dir}.new/"
    chmod 0755 "${dir}.new/rife-ncnn-vulkan"
    rm -rf "${dir}"
    mv "${dir}.new" "${dir}"
    echo "[get-upscalers] ✓  rife -> ${dir} ($(du -sh "${dir}" | cut -f1))"
}

mkdir -p "${BIN_DIR}"
[[ -z "${ONLY}" || "${ONLY}" == realesrgan ]] && install_realesrgan
[[ -z "${ONLY}" || "${ONLY}" == rife ]] && install_rife
echo "[get-upscalers] Done. No restart is needed: the tools look for them each time."
