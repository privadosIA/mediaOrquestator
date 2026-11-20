#!/usr/bin/env bash
# One-command launcher; see scripts/serve.sh (or ./serve.sh --help).
#   ./serve.sh [--local] [--stop]
exec bash "$(dirname "${BASH_SOURCE[0]}")/scripts/serve.sh" "$@"
