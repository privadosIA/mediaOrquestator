#!/usr/bin/env bash
# Runs the installer's detection and system-package steps inside clean containers of
# the supported distributions (no GPU, no network beyond the package mirrors).
#
#   bash scripts/test/docker-distros.sh                 # all images
#   bash scripts/test/docker-distros.sh debian:12       # one image
#
# For each image it runs, as root in a copy of the repository: --check (expected to
# fail on a clean image), --dry-run, --yes --only system, then --check again and
# the versions the toolchain step (go) ends up with.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGES=("$@")
(( ${#IMAGES[@]} )) || IMAGES=(archlinux:latest debian:12 ubuntu:24.04 fedora:41)

inside='
set -u
mkdir -p /work && cd /src
tar cf - --exclude=./bin --exclude=./models --exclude=./voices --exclude=./third_party --exclude=./.git \
    --exclude=./out --exclude=./projects \
    --exclude=./tmp --exclude=./.orchestrator . | tar xf - -C /work
cd /work
# a clean image lacks even the tools the installer itself uses
if   command -v pacman >/dev/null; then pacman -Sy --noconfirm --needed bash awk grep sed coreutils >/dev/null 2>&1
elif command -v apt-get >/dev/null; then apt-get update >/dev/null 2>&1; apt-get install -y gawk >/dev/null 2>&1
elif command -v dnf >/dev/null; then dnf install -y gawk findutils >/dev/null 2>&1; fi
echo "### check (clean image)"; bash scripts/install.sh --check >/tmp/check.out 2>&1; echo "exit=$?"; sed -n "/== Machine/,\$p" /tmp/check.out
echo "### dry-run"; bash scripts/install.sh --dry-run 2>&1 | sed -n "/== Plan/,\$p" | head -30
echo "### install --yes --only system"; bash scripts/install.sh --yes --only system 2>&1 | sed -n "/== Plan/,/== Machine/p" | grep -vE "^\s*$" | tail -40
echo "### check after"; bash scripts/install.sh --check 2>&1 | grep -E "git |cmake|compiler|make |curl|ffmpeg|Go " 
echo "### ffmpeg encoders the pipeline needs"; for e in libx264 aac libmp3lame; do printf "%s: " $e; ffmpeg -hide_banner -encoders 2>/dev/null | grep -c " $e " ; done
echo "### go through the package manager"; bash scripts/install.sh --yes --only go 2>&1 | sed -n "/== Plan/,/== Machine/p" | grep -vE "^\s*$" | tail -14
'
for img in "${IMAGES[@]}"; do
    echo "================ $img"
    docker run --rm -v "$ROOT":/src:ro "$img" bash -c "$inside" 2>&1
done
