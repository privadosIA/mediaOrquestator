#!/usr/bin/env bash
# Tests of scripts/get-whisper.sh with a fake curl (no big files are written).
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

setup() {
    sandbox
    mkdir -p "$IL_ROOT/scripts"
    cp "$TEST_REPO/scripts/get-whisper.sh" "$IL_ROOT/scripts/get-whisper.sh"
    # A download of the wrong size: the script must notice and install nothing.
    stub curl 'out=""; while [[ $# -gt 0 ]]; do [[ "$1" == "-o" ]] && out="$2"; shift; done; printf "tiny" >"$out"'
}

# --- a download of the wrong size ----------------------------------------------------
setup
out="$(bash "$IL_ROOT/scripts/get-whisper.sh" 2>&1)"; st=$?
assert_status "wrong size fails" 1 "$st"
assert_contains "says what it expected" "$out" "expected 574041195 bytes, got 4"
assert_eq "nothing installed" "" "$(ls "$IL_ROOT/models/whisper" 2>/dev/null)"
cleanup_sandbox

# --- a failed download -----------------------------------------------------------------
setup
stub curl 'exit 22'
out="$(bash "$IL_ROOT/scripts/get-whisper.sh" 2>&1)"; st=$?
assert_status "a failed download fails" 22 "$st"
cleanup_sandbox

pass_summary
