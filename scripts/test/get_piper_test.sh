#!/usr/bin/env bash
# Tests of scripts/get-piper.sh with a fake curl: what is downloaded must match its checksum.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

setup() {
    sandbox
    mkdir -p "$IL_ROOT/scripts"
    cp "$TEST_REPO/scripts/get-piper.sh" "$IL_ROOT/scripts/get-piper.sh"
    for tool in sha256sum awk; do ln -sf "$(PATH="$T_REAL_PATH" command -v "$tool")" "$T_BIN/$tool"; done
    # Every download is the same few bytes, which are not what the pinned checksums name.
    stub curl 'out=""; while [[ $# -gt 0 ]]; do [[ "$1" == "-o" ]] && out="$2"; shift; done; printf "tiny" >"$out"'
}

# --- a Piper release that is not the pinned one ------------------------------------------
setup
out="$(bash "$IL_ROOT/scripts/get-piper.sh" 2>&1)"; st=$?
assert_status "a wrong release fails" 1 "$st"
assert_contains "says it does not match" "$out" "does not match its checksum"
assert_eq "nothing installed" "" "$(ls "$IL_ROOT/bin/piper" 2>/dev/null)"
cleanup_sandbox

# --- the voice: a wrong file is removed, an unknown voice needs a checksum ---------------
setup
mkdir -p "$IL_ROOT/bin" && printf '#!/bin/sh\n' >"$IL_ROOT/bin/piper" && chmod +x "$IL_ROOT/bin/piper"
out="$(bash "$IL_ROOT/scripts/get-piper.sh" 2>&1)"; st=$?
assert_status "a wrong voice fails" 1 "$st"
assert_contains "names the voice" "$out" "en_US-lessac-medium.onnx"
assert_eq "the wrong file is gone" "" "$(ls "$IL_ROOT/voices/en_US-lessac-medium.onnx" 2>/dev/null)"
out="$(VOICE_ID=es_ES-davefx-medium bash "$IL_ROOT/scripts/get-piper.sh" 2>&1)"; st=$?
assert_status "an unknown voice has no checksum" 1 "$st"
assert_contains "asks for a checksum" "$out" "no checksum is known"
out="$(VOICE_ID=es_ES-davefx-medium ALLOW_UNVERIFIED=1 bash "$IL_ROOT/scripts/get-piper.sh" 2>&1)"; st=$?
assert_status "unverified on request" 0 "$st"
assert_contains "warns" "$out" "unverified"
assert_eq "installed on request" "tiny" "$(cat "$IL_ROOT/voices/es_ES-davefx-medium.onnx")"
cleanup_sandbox

pass_summary
