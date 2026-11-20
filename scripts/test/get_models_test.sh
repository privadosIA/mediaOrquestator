#!/usr/bin/env bash
# Tests of scripts/get-models.sh with a fake curl: the checkpoint must match its checksum.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

setup() {
    sandbox
    mkdir -p "$IL_ROOT/scripts"
    cp "$TEST_REPO/scripts/get-models.sh" "$IL_ROOT/scripts/get-models.sh"
    for tool in sha256sum awk; do ln -sf "$(PATH="$T_REAL_PATH" command -v "$tool")" "$T_BIN/$tool"; done
    stub curl 'out=""; while [[ $# -gt 0 ]]; do [[ "$1" == "-o" ]] && out="$2"; shift; done; printf "tiny" >"$out"'
}

setup
out="$(bash "$IL_ROOT/scripts/get-models.sh" 2>&1)"; st=$?
assert_status "a wrong checkpoint fails" 1 "$st"
assert_contains "says it does not match" "$out" "does not match its checksum"
assert_eq "nothing installed" "" "$(ls "$IL_ROOT/models" 2>/dev/null)"
out="$(MODEL_NAME=other.safetensors MODEL_URL=https://example.test/o.safetensors bash "$IL_ROOT/scripts/get-models.sh" 2>&1)"; st=$?
assert_status "another model has no checksum" 1 "$st"
assert_contains "asks for a checksum" "$out" "no checksum is known"
want="$(printf tiny | sha256sum | awk '{print $1}')"
out="$(MODEL_NAME=other.safetensors MODEL_URL=https://example.test/o.safetensors MODEL_SHA256="$want" bash "$IL_ROOT/scripts/get-models.sh" 2>&1)"; st=$?
assert_status "with its checksum it installs" 0 "$st"
assert_eq "installed" "tiny" "$(cat "$IL_ROOT/models/other.safetensors")"
cleanup_sandbox

pass_summary
