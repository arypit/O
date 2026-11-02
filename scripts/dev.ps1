# O dev (Windows): run the engine via go run with args passed through
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
go run ./cmd/o @args
