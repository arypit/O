#!/usr/bin/env bash
# O start (POSIX): run the built engine binary, building it first if missing
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
[ -x bin/o ] || "$root/scripts/build.sh"
./bin/o "$@"
