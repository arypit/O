# O build (Windows): compile the Go engine (native full-screen TUI included) to bin/o.exe.
# The native TUI is part of the binary - Node is NOT required.
# Usage:
#   .\scripts\build.ps1             build the native binary only (default, no Node needed)
#   .\scripts\build.ps1 -WithNodeTUI also build the older TypeScript TUI (tui/dist)
param([switch]$WithNodeTUI)
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
go build -o bin/o.exe ./cmd/o
if ($WithNodeTUI -and (Test-Path "$root/tui/package.json")) {
  Push-Location "$root/tui"
  if (-not (Test-Path "$root/tui/node_modules")) { npm install }
  npx tsc -p tsconfig.json
  Pop-Location
  Write-Host "built bin/o.exe and tui/dist"
} else {
  Write-Host "built bin/o.exe (native TUI included)"
}
