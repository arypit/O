#!/usr/bin/env bash
# O install (POSIX): build the engine + TUI and print next steps.
# Usage:  ./scripts/install.sh            build only
#         ./scripts/install.sh --add-to-path   also symlink bin/o into ~/.local/bin
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"
"$root/scripts/build.sh"
if [ "${1:-}" = "--add-to-path" ]; then
  mkdir -p "$HOME/.local/bin"
  ln -sf "$root/bin/o" "$HOME/.local/bin/o"
  echo "symlinked $root/bin/o -> ~/.local/bin/o (make sure ~/.local/bin is on your PATH)"
fi
version="$(cat "$root/VERSION")"
echo ""
echo "O $version installed."
echo "Next steps:"
echo "  o version                        # or: $root/bin/o version"
echo "  o login                          # add a provider: id, base URL, API key, model"
echo "  o run -p \"your prompt\"         # one-shot agent run"
echo "  $root/scripts/tui.sh \"your prompt\"   # interactive TUI"
echo "  $root/scripts/tui.sh /login            # provider manager from the TUI"
echo ""
echo "The built-in \"adaop\" provider uses https://api.adaop.sbs/v1 and the ADAOP_API_KEY"
echo "environment variable; providers added with 'o login' are saved to ~/.o/config.json"
echo "(owner-only permissions) and usable with --provider <id>."
