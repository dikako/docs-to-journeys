#!/usr/bin/env bash
# docs-to-journeys launcher. Builds the Go CLI (cmd/journeys) when its sources
# change, then forwards every argument. Run it from any folder: reports/,
# work/, features/ and generated/ are created in the current directory.
#
#   ./run_journeys.sh all examples/login-prd.md emulator-5554
#   ./run_journeys.sh run emulator-5554 examples/journeys
#   ./run_journeys.sh --help
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN="$ROOT/bin/journeys"

command -v go >/dev/null || { echo "error: Go is required (https://go.dev/dl/)" >&2; exit 2; }

if [[ ! -x "$BIN" ]] || [[ -n "$(find "$ROOT/cmd" "$ROOT/internal" "$ROOT/go.mod" "$ROOT/go.sum" -newer "$BIN" -print -quit)" ]]; then
  (cd "$ROOT" && go build -o "$BIN" ./cmd/journeys)
fi

# Use the repo's sources.yaml unless the caller points elsewhere.
export SOURCES_FILE="${SOURCES_FILE:-$ROOT/sources.yaml}"

exec "$BIN" "$@"
