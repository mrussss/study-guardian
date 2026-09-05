param([string]$RootDir, [switch]$EnableAutostart)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
& (Join-Path $PSScriptRoot "set-desktop-shortcut.ps1") -Create -RootDir $RootDir | Out-Null
if ($EnableAutostart) { & (Join-Path $PSScriptRoot "set-autostart.ps1") -Enable -RootDir $RootDir | Out-Null }
Write-Host "StudyGuardian Windows integration installed." -ForegroundColor Green
