#!/bin/sh
# O TUI: build the engine if needed, then run the NATIVE full-screen TUI.
# The TUI is compiled into the o binary (no Node needed).
set -e
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
[ -x "$root/bin/o" ] || go build -o bin/o ./cmd/o
exec "$root/bin/o" tui "$@"
