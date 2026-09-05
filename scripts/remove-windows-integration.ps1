param([string]$RootDir)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
& (Join-Path $PSScriptRoot "set-desktop-shortcut.ps1") -Remove -RootDir $RootDir | Out-Null
& (Join-Path $PSScriptRoot "set-autostart.ps1") -Disable -RootDir $RootDir | Out-Null
Write-Host "StudyGuardian Windows integration removed." -ForegroundColor Green
