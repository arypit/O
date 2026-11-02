#!/usr/bin/env bash
# O dev (POSIX): run the engine via go run with args passed through
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
go run ./cmd/o "$@"
