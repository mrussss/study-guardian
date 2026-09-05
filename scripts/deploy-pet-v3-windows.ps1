[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$ArtifactPath = "D:\StudyGuardianBuild\artifacts\pet-v3\debug\StudyGuardian.exe",
    [string]$RuntimeExe = "D:\StudyGuardianDev\pet-v3\StudyGuardian.exe",
    [string]$BackupRoot = "D:\StudyGuardianBuild\backups\pet-v3",
    [switch]$VerifyOnly,
    [switch]$NoStart,
    [switch]$TestMode,
    [switch]$SimulateFailureAfterReplace
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$expectedRuntimeExe = [IO.Path]::GetFullPath("D:\StudyGuardianDev\pet-v3\StudyGuardian.exe")
$testRuntimeRoot = [IO.Path]::GetFullPath("D:\StudyGuardianBuild\test-runtime")
$expectedArtifactRoot = [IO.Path]::GetFullPath("D:\StudyGuardianBuild\artifacts\pet-v3")
$expectedBackupRoot = [IO.Path]::GetFullPath("D:\StudyGuardianBuild\backups\pet-v3")
$ArtifactPath = [IO.Path]::GetFullPath($ArtifactPath)
$RuntimeExe = [IO.Path]::GetFullPath($RuntimeExe)
$BackupRoot = [IO.Path]::GetFullPath($BackupRoot)
$runtimeDirectory = Split-Path -Parent $RuntimeExe
$timestamp = Get-Date -Format "yyyyMMdd-HHmmssfff"
$backupPath = Join-Path $BackupRoot "StudyGuardian-$timestamp.exe"
$temporaryPath = Join-Path $runtimeDirectory "StudyGuardian.exe.deploy-$timestamp.tmp"
$oldHash = $null
$newHash = $null
$rollbackRequired = $false
$wasRunning = $false

function Get-Sha256 {
    param([Parameter(Mandatory)] [string]$Path)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Path)
    try {
        return ([BitConverter]::ToString($sha256.ComputeHash($stream))).Replace("-", "")
    } finally {
        $stream.Dispose()
        $sha256.Dispose()
    }
}

if ($TestMode) {
    foreach ($path in @($ArtifactPath, $RuntimeExe, $BackupRoot)) {
        if (-not ($path.StartsWith("$testRuntimeRoot\", [StringComparison]::OrdinalIgnoreCase))) {
            throw "Test paths must remain under $testRuntimeRoot"
        }
    }
    if (-not $NoStart) { throw "TestMode requires -NoStart" }
} elseif ($RuntimeExe -ne $expectedRuntimeExe) {
    throw "Production runtime path must be exactly $expectedRuntimeExe"
} elseif (-not $ArtifactPath.StartsWith("$expectedArtifactRoot\", [StringComparison]::OrdinalIgnoreCase)) {
    throw "Production artifact must remain under $expectedArtifactRoot"
} elseif (-not ($BackupRoot -eq $expectedBackupRoot -or $BackupRoot.StartsWith("$expectedBackupRoot\", [StringComparison]::OrdinalIgnoreCase))) {
    throw "Production backups must remain under $expectedBackupRoot"
}
if ($SimulateFailureAfterReplace -and -not $TestMode) { throw "Failure simulation is available only in TestMode" }

function Get-ExactRuntimeProcesses {
    if ($TestMode) { return @() }
    @(Get-CimInstance Win32_Process -Filter "Name = 'StudyGuardian.exe'" | Where-Object {
        $_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath) -eq $RuntimeExe
    })
}

function Wait-ForRuntimeExit {
    param([int]$TimeoutSeconds = 30)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        if (@(Get-ExactRuntimeProcesses).Count -eq 0) { return }
        Start-Sleep -Milliseconds 250
    } while ((Get-Date) -lt $deadline)
    throw "StudyGuardian.exe did not exit within ${TimeoutSeconds}s"
}

function Wait-ForExclusiveFile {
    param([Parameter(Mandatory)] [string]$Path, [int]$TimeoutSeconds = 30)
    if (-not (Test-Path -LiteralPath $Path)) { return }
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $stream = $null
        try {
            $stream = [IO.File]::Open($Path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
            return
        } catch {
            Start-Sleep -Milliseconds 250
        } finally {
            if ($null -ne $stream) { $stream.Dispose() }
        }
    } while ((Get-Date) -lt $deadline)
    throw "File handle was not released: $Path"
}

function Start-And-VerifyRuntime {
    $started = Start-Process -FilePath $RuntimeExe -WorkingDirectory $runtimeDirectory -WindowStyle Hidden -PassThru
    $deadline = (Get-Date).AddSeconds(20)
    do {
        Start-Sleep -Milliseconds 250
        $matches = @(Get-ExactRuntimeProcesses)
        if ($matches.Count -eq 1) {
            $actualHash = Get-Sha256 -Path $RuntimeExe
            if ($actualHash -ne $newHash) { throw "Running executable hash does not match the deployed artifact" }
            $process = Get-Process -Id $matches[0].ProcessId
            Write-Host "Runtime PID $($process.Id); StartTime $($process.StartTime.ToString('o')); SHA256 $actualHash"
            return
        }
    } while ((Get-Date) -lt $deadline)
    if ($started.HasExited) { throw "New StudyGuardian.exe exited with code $($started.ExitCode)" }
    throw "New StudyGuardian.exe did not appear at the expected path"
}

if (-not (Test-Path -LiteralPath $ArtifactPath -PathType Leaf)) { throw "Artifact does not exist: $ArtifactPath" }
$newHash = Get-Sha256 -Path $ArtifactPath
if ($VerifyOnly) {
    $runtimeHash = if (Test-Path -LiteralPath $RuntimeExe -PathType Leaf) { Get-Sha256 -Path $RuntimeExe } else { "missing" }
    $processes = @(Get-ExactRuntimeProcesses)
    Write-Host "Artifact SHA256: $newHash"
    Write-Host "Runtime SHA256: $runtimeHash"
    Write-Host "Matching runtime processes: $($processes.Count)"
    if ($runtimeHash -ne $newHash) { throw "Runtime and artifact hashes differ" }
    if (-not $TestMode -and $processes.Count -ne 1) { throw "Expected one running StudyGuardian process" }
    return
}

$processes = @(Get-ExactRuntimeProcesses)
if ($processes.Count -gt 1) { throw "Multiple StudyGuardian processes are running from the production path" }
$wasRunning = $processes.Count -eq 1
if (-not $PSCmdlet.ShouldProcess($RuntimeExe, "backup, replace, and start the Pet v3 executable")) { return }

try {
    if ($wasRunning) {
        Stop-Process -Id $processes[0].ProcessId -Force
        Wait-ForRuntimeExit
    }
    Wait-ForExclusiveFile -Path $RuntimeExe
    New-Item -ItemType Directory -Path $runtimeDirectory, $BackupRoot -Force | Out-Null
    if (Test-Path -LiteralPath $RuntimeExe -PathType Leaf) {
        $oldHash = Get-Sha256 -Path $RuntimeExe
        Copy-Item -LiteralPath $RuntimeExe -Destination $backupPath -Force
        if ((Get-Sha256 -Path $backupPath) -ne $oldHash) { throw "Backup hash mismatch" }
        $rollbackRequired = $true
    }
    Copy-Item -LiteralPath $ArtifactPath -Destination $temporaryPath -Force
    if ((Get-Sha256 -Path $temporaryPath) -ne $newHash) { throw "Temporary artifact hash mismatch" }
    Move-Item -LiteralPath $temporaryPath -Destination $RuntimeExe -Force
    if ((Get-Sha256 -Path $RuntimeExe) -ne $newHash) { throw "Runtime hash mismatch after replacement" }
    if ($SimulateFailureAfterReplace) { throw "Simulated post-replacement failure" }
    if (-not $NoStart) { Start-And-VerifyRuntime }
    $rollbackRequired = $false

    $backups = @(Get-ChildItem -LiteralPath $BackupRoot -Filter "StudyGuardian-*.exe" -File | Sort-Object LastWriteTime -Descending)
    foreach ($oldBackup in ($backups | Select-Object -Skip 3)) { Remove-Item -LiteralPath $oldBackup.FullName -Force }
    Write-Host "Pet deployment succeeded. SHA256: $newHash"
    if ($oldHash) { Write-Host "Previous SHA256: $oldHash; Backup: $backupPath" }
} catch {
    $failure = $_
    if (Test-Path -LiteralPath $temporaryPath) { Remove-Item -LiteralPath $temporaryPath -Force -ErrorAction SilentlyContinue }
    if ($rollbackRequired -and (Test-Path -LiteralPath $backupPath -PathType Leaf)) {
        Copy-Item -LiteralPath $backupPath -Destination $RuntimeExe -Force
        $restoredHash = Get-Sha256 -Path $RuntimeExe
        if ($restoredHash -ne $oldHash) { throw "Rollback hash mismatch after: $($failure.Exception.Message)" }
        if ($wasRunning -and -not $NoStart -and -not $TestMode) {
            Start-Process -FilePath $RuntimeExe -WorkingDirectory $runtimeDirectory -WindowStyle Hidden | Out-Null
        }
        Write-Host "Rollback restored the previous StudyGuardian executable."
    }
    throw $failure
}
