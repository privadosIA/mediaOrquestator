#!/usr/bin/env bash
# Tests of scripts/check-docs.sh on small trees.
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
CHECK="$TEST_REPO/scripts/check-docs.sh"

tree() { # writes files given as "path<<<content" args
    local spec p c
    for spec in "$@"; do
        p="${spec%%<<<*}"; c="${spec#*<<<}"
        _inside "$T_DIR/tree/$p"
        mkdir -p "$(dirname "$T_DIR/tree/$p")"
        printf '%b' "$c" >"$T_DIR/tree/$p"
    done
}
run() { out="$(bash "$CHECK" "$T_DIR/tree" 2>&1)"; st=$?; }

# --- a clean tree ----------------------------------------------------------------
sandbox
tree 'README.md<<<# Title\n\nSee [install](docs/INSTALL.md#the-setup), [cli](docs/CLI.md), [self](#title), ![shot](docs/img.png), [web](https://example.com/x) and [mail](mailto:a@b.c).\n' \
     'docs/INSTALL.md<<<# Install\n\n## The setup\n\n## Ñandú y café\n\nBack to [readme](../README.md#title) and [cafe](#ñandú-y-café).\n' \
     'docs/CLI.md<<<# CLI\n\n## Repeated\n\n## Repeated\n\n[second](#repeated-1)\n' \
     'docs/img.png<<<x'
run
assert_status "clean tree" 0 "$st"
assert_contains "clean message" "$out" "every relative link resolves"
cleanup_sandbox

# --- a missing file -----------------------------------------------------------------
sandbox
tree 'README.md<<<# Title\n\n[nope](docs/NOPE.md)\n' 'docs/A.md<<<# A\n'
run
assert_status "missing file" 1 "$st"
assert_contains "names the file and the link" "$out" "README.md:3: broken link 'docs/NOPE.md'"
cleanup_sandbox

# --- a missing heading -----------------------------------------------------------------
sandbox
tree 'README.md<<<# Title\n\n[x](docs/A.md#not-there)\n' 'docs/A.md<<<# A\n\n## Real\n'
run
assert_status "missing heading" 1 "$st"
assert_contains "names the heading" "$out" "no heading '#not-there'"
cleanup_sandbox

# --- a missing image and a link in the same file as the anchor ---------------------------
sandbox
tree 'README.md<<<# Title\n\n![shot](docs/gone.png) and [here](#missing-here)\n' 'docs/A.md<<<# A\n'
run
assert_status "missing image and same-file anchor" 1 "$st"
assert_contains "image" "$out" "docs/gone.png"
assert_contains "same-file anchor" "$out" "#missing-here"
cleanup_sandbox

# --- code is not checked ------------------------------------------------------------------
sandbox
tree 'README.md<<<# Title\n\n```md\n[fenced](docs/NOPE.md)\n# Not a heading [x](nope.md)\n```\n\nInline `[code](nope.md)` too.\n\n[ok](#title)\n'
run
assert_status "links in code are ignored" 0 "$st"
cleanup_sandbox

# --- the real repository --------------------------------------------------------------------
out="$(bash "$CHECK" "$TEST_REPO" 2>&1)"; st=$?
assert_status "the repository's own docs" 0 "$st"

pass_summary
