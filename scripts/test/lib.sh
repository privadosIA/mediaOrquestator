#!/usr/bin/env bash
# Tiny test harness for the shell scripts. Source it from a test file.
#
#   sandbox            creates a scratch directory with a PATH that holds only the
#                      essential coreutils plus the stubs a test adds
#   stub NAME 'BODY'   puts an executable NAME into the sandbox PATH
#   assert_eq / assert_contains / assert_not_contains / assert_status / pass_summary

set -uo pipefail

TEST_REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
T_REAL_PATH="$PATH"   # restored at the start of every sandbox: the stub PATH of the previous one is gone by then
T_FAILS=0
T_COUNT=0

# Real commands the scripts need (everything else is stubbed per test).
T_ESSENTIAL=(bash sh grep sed awk sort head tail tr cat df uname id date mkdir tee cut dirname basename ls rm cp mv touch env wc seq tty sleep timeout stat readlink chmod ln mktemp find xargs sync cmp diff expr false true test)

sandbox() {
    PATH="$T_REAL_PATH"
    T_DIR="$(mktemp -d "${TMPDIR:-/tmp}/mo-test.XXXXXX")" || { echo "sandbox: mktemp failed" >&2; exit 3; }
    # Everything the tests write must stay under this directory: refuse anything else.
    [[ -n "$T_DIR" && "$T_DIR" == /tmp/* || "$T_DIR" == "${TMPDIR:-/tmp}"/* ]] || { echo "sandbox: unsafe directory '$T_DIR'" >&2; exit 3; }
    T_BIN="$T_DIR/bin"
    mkdir -p "$T_BIN" "$T_DIR/home"
    local c p
    for c in "${T_ESSENTIAL[@]}"; do
        p="$(command -v "$c" 2>/dev/null || true)"
        [[ -n "$p" && "$p" == /* ]] && ln -sf "$p" "$T_BIN/$c"
    done
    export PATH="$T_BIN"
    export HOME="$T_DIR/home"
    export IL_LOG_DIR="$T_DIR/log"
    unset IL_LOG
    export IL_ROOT="$T_DIR/repo"
    mkdir -p "$IL_ROOT/scripts" "$IL_ROOT/bin" "$IL_ROOT/voices" "$IL_ROOT/models"
}

_inside() { [[ -n "${T_DIR:-}" && "$1" == "$T_DIR"/* ]] || { echo "test harness: refusing to touch '$1' outside the sandbox" >&2; exit 3; }; }

stub() { # name body
    _inside "$T_BIN/$1"
    printf '#!/usr/bin/env bash\n%s\n' "$2" >"$T_BIN/$1"
    chmod +x "$T_BIN/$1"
}

# fake_repo_file PATH [BODY]: a file (executable when it starts with #!) in the fake repo.
fake_repo_file() {
    _inside "$IL_ROOT/$1"
    mkdir -p "$(dirname "$IL_ROOT/$1")"
    printf '%s\n' "${2:-x}" >"$IL_ROOT/$1"
    [[ "${2:-}" == '#!'* ]] && chmod +x "$IL_ROOT/$1"
    return 0
}

_fail() { T_FAILS=$((T_FAILS + 1)); echo "  FAIL: $1" >&2; }

assert_eq() { # name expected actual
    T_COUNT=$((T_COUNT + 1))
    [[ "$2" == "$3" ]] || _fail "$1: expected [$2], got [$3]"
}
assert_contains() { # name haystack needle
    T_COUNT=$((T_COUNT + 1))
    [[ "$2" == *"$3"* ]] || _fail "$1: [$3] not found in: ${2:0:400}"
}
assert_not_contains() {
    T_COUNT=$((T_COUNT + 1))
    [[ "$2" != *"$3"* ]] || _fail "$1: [$3] unexpectedly found in: ${2:0:400}"
}
assert_status() { # name expected actual
    assert_eq "$1 (status)" "$2" "$3"
}

pass_summary() {
    if (( T_FAILS == 0 )); then echo "ok: $T_COUNT assertions"; return 0; fi
    echo "FAILED: $T_FAILS of $T_COUNT assertions" >&2
    return 1
}

cleanup_sandbox() {
    if [[ -n "${T_DIR:-}" && "$T_DIR" == */mo-test.* ]]; then rm -rf "$T_DIR"; fi
    PATH="$T_REAL_PATH"
}
