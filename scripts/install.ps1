# O install (Windows): build the engine + TUI, optionally put o.exe on the user PATH.
# Usage:  .\scripts\install.ps1            build only
#         .\scripts\install.ps1 -AddToPath  build + add <O>\bin to the user PATH
param([switch]$AddToPath)
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root
& "$root/scripts/build.ps1"
if ($AddToPath) {
  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  if ($userPath -notlike "*$root\bin*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$root\bin", "User")
    Write-Host "added $root\bin to the user PATH (open a new terminal to use it)"
  } else {
    Write-Host "$root\bin is already on the user PATH"
  }
}
Write-Host @"

O $((Get-Content "$root\VERSION") -join '') installed.

Next steps (open a NEW terminal so the PATH change applies):
  o                                      # type `o` from ANY directory -> full-screen TUI opens
  o tui --theme light                    # same TUI, light mode (dark is default)
  o login                                # add a provider: id, base URL, API key, model
  o run -p "your prompt"                 # one-shot agent run (no TUI)
  o tui --node                           # the older TypeScript TUI, if you prefer it

The built-in "adaop" provider uses https://api.adaop.sbs/v1 and the ADAOP_API_KEY
environment variable; any custom provider you add with `o login` is saved to
~/.o/config.json (owner-only permissions) and usable with --provider <id>.
"@
