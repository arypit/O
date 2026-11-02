# O TUI (Windows): build the engine if needed, then run the NATIVE full-screen TUI.
# The TUI is compiled into o.exe (no Node needed).
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
if (-not (Test-Path "$root/bin/o.exe")) { go build -o bin/o.exe ./cmd/o }
& "$root/bin/o.exe" tui @args
