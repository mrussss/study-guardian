Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$deployScript = Join-Path $repoRoot "scripts\deploy-pet-v3-windows.ps1"
$testBase = [IO.Path]::GetFullPath("D:\StudyGuardianBuild\test-runtime")
$testRoot = Join-Path $testBase ([guid]::NewGuid().ToString("N"))
$artifact = Join-Path $testRoot "artifact\StudyGuardian.exe"
$runtime = Join-Path $testRoot "runtime\StudyGuardian.exe"
$backups = Join-Path $testRoot "backups"

function Assert-FileContent {
    param([string]$Path, [string]$Expected)
    $actual = [IO.File]::ReadAllText($Path)
    if ($actual -ne $Expected) { throw "Unexpected content at ${Path}: $actual" }
}

try {
    New-Item -ItemType Directory -Path (Split-Path -Parent $artifact), (Split-Path -Parent $runtime), $backups -Force | Out-Null
    [IO.File]::WriteAllText($runtime, "old-runtime")
    [IO.File]::WriteAllText($artifact, "new-runtime")

    & $deployScript -ArtifactPath $artifact -RuntimeExe $runtime -BackupRoot $backups -TestMode -NoStart
    Assert-FileContent -Path $runtime -Expected "new-runtime"
    $firstBackup = @(Get-ChildItem -LiteralPath $backups -Filter "StudyGuardian-*.exe" -File)
    if ($firstBackup.Count -ne 1) { throw "Expected one deployment backup" }
    Assert-FileContent -Path $firstBackup[0].FullName -Expected "old-runtime"

    [IO.File]::WriteAllText($artifact, "whatif-runtime")
    & $deployScript -ArtifactPath $artifact -RuntimeExe $runtime -BackupRoot $backups -TestMode -NoStart -WhatIf
    Assert-FileContent -Path $runtime -Expected "new-runtime"

    [IO.File]::WriteAllText($runtime, "rollback-runtime")
    [IO.File]::WriteAllText($artifact, "broken-runtime")
    $failed = $false
    try {
        & $deployScript -ArtifactPath $artifact -RuntimeExe $runtime -BackupRoot $backups -TestMode -NoStart -SimulateFailureAfterReplace
    } catch {
        $failed = $true
    }
    if (-not $failed) { throw "Expected simulated deployment failure" }
    Assert-FileContent -Path $runtime -Expected "rollback-runtime"

    $outsideRuntime = Join-Path "D:\StudyGuardianBuild" "outside-test\StudyGuardian.exe"
    $guarded = $false
    try {
        & $deployScript -ArtifactPath $artifact -RuntimeExe $outsideRuntime -BackupRoot $backups -TestMode -NoStart
    } catch {
        $guarded = $true
    }
    if (-not $guarded) { throw "TestMode accepted a path outside its allowlisted root" }

    Write-Host "PASS: Pet v3 deployment, WhatIf, rollback, and path guards"
} finally {
    if (Test-Path -LiteralPath $testRoot) { [IO.Directory]::Delete($testRoot, $true) }
}
