[CmdletBinding(SupportsShouldProcess)]
param([switch]$BuildOnly)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot "..\.."))
$buildScript = Join-Path $repoRoot "scripts\build-pet-v3-windows.ps1"
$deployScript = Join-Path $repoRoot "scripts\deploy-pet-v3-windows.ps1"

Write-Warning "This compatibility entry point now uses the persistent D:\StudyGuardianBuild cache. Prefer ./scripts/pet-v3.sh candidate from WSL."
& $buildScript -RepoRoot $repoRoot -Configuration Debug -RunFrontendTests -RunRustTests
if (-not $BuildOnly -and -not $WhatIfPreference) {
    & $deployScript
} else {
    Write-Host "Build complete; deployment skipped."
}
