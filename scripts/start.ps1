# O start (Windows): run the built engine binary, building it first if missing
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
if (-not (Test-Path "$root/bin/o.exe")) { & "$root/scripts/build.ps1" }
& "$root/bin/o.exe" @args
