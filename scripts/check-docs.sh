#!/usr/bin/env bash
# scripts/check-docs.sh
#
# Checks every relative Markdown link and #anchor in README.md and docs/*.md.
# Exits 0 when all resolve, 1 (naming the file and the link) when a target file
# or heading does not exist. External links (http, https, mailto) are not fetched.
#
#   bash scripts/check-docs.sh [root]      # root defaults to the project directory

set -uo pipefail
export LC_ALL="${LC_ALL:-C.UTF-8}"

ROOT="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
cd "$ROOT" || exit 2

# headings FILE: the GitHub-style anchors of a file, one per line (code fences skipped,
# duplicates numbered -1, -2 like the renderers do).
headings() {
    awk '
        /^```/ || /^~~~/ { fence = !fence; next }
        fence { next }
        /^#+[ \t]/ {
            h = $0
            sub(/^#+[ \t]+/, "", h)
            gsub(/[ \t]+#+[ \t]*$/, "", h)
            gsub(/`/, "", h)
            gsub(/\[([^]]*)\]\([^)]*\)/, "\\1", h)   # [text](url) -> text
            h = tolower(h)
            gsub(/[^[:alnum:] _-]/, "", h)
            gsub(/ /, "-", h)
            n = seen[h]++
            print (n ? h "-" n : h)
        }
    ' "$1"
}

# links FILE: "line<TAB>target" for every inline link outside code fences and spans.
links() {
    awk '
        /^```/ || /^~~~/ { fence = !fence; next }
        fence { next }
        {
            line = $0
            gsub(/`[^`]*`/, "", line)               # inline code is not a link
            while (match(line, /\]\([^) ]+[^)]*\)/)) {
                t = substr(line, RSTART + 2, RLENGTH - 3)
                sub(/[ \t]+".*$/, "", t)              # [x](url "title")
                print NR "\t" t
                line = substr(line, RSTART + RLENGTH)
            }
        }
    ' "$1"
}

bad=0
files=(README.md docs/*.md)
for f in "${files[@]}"; do
    [[ -f "$f" ]] || continue
    dir="$(dirname "$f")"
    while IFS=$'\t' read -r ln target; do
        case "$target" in
            http://*|https://*|mailto:*|"") continue ;;
        esac
        path="${target%%#*}"
        anchor=""
        [[ "$target" == *"#"* ]] && anchor="${target#*#}"
        if [[ -z "$path" ]]; then dest="$f"; else dest="$dir/$path"; fi
        if [[ ! -e "$dest" ]]; then
            echo "$f:$ln: broken link '$target' (no such file: $dest)"
            bad=1
            continue
        fi
        if [[ -n "$anchor" && "$dest" == *.md ]]; then
            if ! headings "$dest" | grep -qxF -- "$anchor"; then
                echo "$f:$ln: broken link '$target' (no heading '#$anchor' in $dest)"
                bad=1
            fi
        fi
    done < <(links "$f")
done

# The routes a skill names must exist: every `METHOD /api/path` in .claude/skills/*/SKILL.md has its path and its method in
# internal/server/openapi.yaml (the path as written there, with its {parameters}).
OPENAPI="${OPENAPI:-internal/server/openapi.yaml}"
if [[ -f "$OPENAPI" ]]; then
    for skill in .claude/skills/*/SKILL.md; do
        [[ -f "$skill" ]] || continue
        while IFS=$'\t' read -r ln method route; do
            found="$(awk -v route="$route" -v method="$(echo "$method" | tr 'A-Z' 'a-z')" '
                $0 == "  " route ":" { inroute = 1; next }
                /^  \// { inroute = 0 }
                inroute && $0 == "    " method ":" { print "yes"; exit }
            ' "$OPENAPI")"
            if [[ "$found" != "yes" ]]; then
                echo "$skill:$ln: the skill names '$method $route', which is not in $OPENAPI"
                bad=1
            fi
        done < <(grep -nE '`(GET|POST|PUT|PATCH|DELETE) /api/[A-Za-z0-9_{}./-]+' "$skill" | sed -E 's/^([0-9]+):.*/\1/' | sort -un | while read -r n; do
            sed -n "${n}p" "$skill" | grep -oE '`(GET|POST|PUT|PATCH|DELETE) /api/[A-Za-z0-9_{}./-]+' | tr -d '`' | while read -r m r; do printf '%s\t%s\t%s\n' "$n" "$m" "$r"; done
        done)
    done
fi

if (( bad )); then exit 1; fi
echo "docs: every relative link resolves."
