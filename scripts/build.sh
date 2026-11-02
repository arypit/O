#!/usr/bin/env bash
# O build (POSIX): compile the Go engine (native full-screen TUI included) to bin/o.
# The native TUI is part of the binary - Node is NOT required.
# Usage:
#   ./scripts/build.sh             build the native binary only (default, no Node needed)
#   ./scripts/build.sh --with-node also build the older TypeScript TUI (tui/dist)
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
go build -o bin/o ./cmd/o
with_node=0
for arg in "$@"; do
  [ "$arg" = "--with-node" ] && with_node=1
done
if [ "$with_node" = "1" ] && [ -f tui/package.json ]; then
  (cd tui && { [ -d node_modules ] || npm install; } && npx tsc -p tsconfig.json)
  echo "built bin/o and tui/dist"
else
  echo "built bin/o (native TUI included)"
fi
