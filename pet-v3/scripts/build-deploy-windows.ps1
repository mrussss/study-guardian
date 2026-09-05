[CmdletBinding(SupportsShouldProcess)]
param(
  [switch]$BuildOnly,
  [ValidateNotNullOrEmpty()]
  [string]$Distro = "Ubuntu-22.04",
  [ValidateNotNullOrEmpty()]
  [string]$WslRepo = "/home/lls/projects/study-guardian/pet-v3",
  [string]$BuildRoot = (Join-Path ([IO.Path]::GetTempPath()) "studyguardian-pet-v3-builds"),
  [string]$RuntimeDir = "D:\StudyGuardianDev\pet-v3"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$expectedRuntimeExe = "D:\StudyGuardianDev\pet-v3\StudyGuardian.exe"
$timestamp = Get-Date -Format "yyyyMMdd-HHmmssfff"
$buildDir = Join-Path $BuildRoot ("pet-v3-" + $timestamp)
$sourcePath = "\\wsl.localhost\$Distro" + ($WslRepo -replace "/", "\")
$sourceParent = Split-Path -Parent $sourcePath
$assetSourcePath = Join-Path $sourceParent "pet\assets"
$assetBuildPath = Join-Path (Split-Path -Parent $buildDir) "pet\assets"
$targetDir = Join-Path $buildDir "src-tauri\target"
$builtExe = Join-Path $targetDir "debug\studyguardian-pet-v3.exe"
$runtimeExe = Join-Path $RuntimeDir "StudyGuardian.exe"
$backupPath = Join-Path $RuntimeDir ("StudyGuardian.exe.pre-taskpicker-" + $timestamp + ".exe")
$temporaryExe = $runtimeExe + ".deploy-" + $timestamp + ".tmp"
$rollbackRequired = $false
$oldHash = $null
$newHash = $null

function Invoke-Checked {
  param(
    [Parameter(Mandatory)] [string]$FilePath,
    [Parameter(Mandatory)] [string[]]$Arguments,
    [Parameter(Mandatory)] [string]$WorkingDirectory,
    [Parameter(Mandatory)] [string]$Label
  )
  Write-Host ("==> " + $Label)
  Push-Location $WorkingDirectory
  try {
    & $FilePath @Arguments
    $exitCode = $LASTEXITCODE
  } finally {
    Pop-Location
  }
  if ($exitCode -ne 0) {
    throw ("{0} failed with exit code {1}" -f $Label, $exitCode)
  }
}

function Get-RuntimeProcesses {
  @(Get-CimInstance Win32_Process -Filter "Name = 'StudyGuardian.exe'" | Where-Object {
      $_.ExecutablePath -eq $expectedRuntimeExe
    })
}

function Wait-ForRuntimeExit {
  param([int]$TimeoutSeconds = 30)
  $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
  do {
    $processes = @(Get-RuntimeProcesses)
    if ($processes.Count -eq 0) { return }
    Start-Sleep -Milliseconds 250
  } while ((Get-Date) -lt $deadline)
  throw "StudyGuardian.exe did not exit within ${TimeoutSeconds}s"
}

function Wait-ForExclusiveFile {
  param(
    [Parameter(Mandatory)] [string]$Path,
    [int]$TimeoutSeconds = 30
  )
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
  throw "File handle for $Path was not released within ${TimeoutSeconds}s"
}

function Stop-ExactRuntime {
  $processes = @(Get-RuntimeProcesses)
  if ($processes.Count -ne 1) {
    throw ("Expected exactly one runtime process at {0}; found {1}" -f $expectedRuntimeExe, $processes.Count)
  }
  Write-Host ("Stopping verified process PID {0}: {1}" -f $processes[0].ProcessId, $processes[0].ExecutablePath)
  Stop-Process -Id $processes[0].ProcessId -Force
  Wait-ForRuntimeExit
  Wait-ForExclusiveFile -Path $runtimeExe
}

function Start-And-VerifyRuntime {
  $started = Start-Process -FilePath $runtimeExe -WorkingDirectory $RuntimeDir -PassThru
  $deadline = (Get-Date).AddSeconds(20)
  do {
    Start-Sleep -Milliseconds 250
    $matches = @(Get-RuntimeProcesses)
    if ($matches.Count -eq 1) {
      $process = Get-Process -Id $matches[0].ProcessId
      $actualHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash.ToUpperInvariant()
      if ($actualHash -ne $newHash) {
        throw ("Running executable hash {0} does not match build hash {1}" -f $actualHash, $newHash)
      }
      Write-Host ("Runtime PID {0}; Path {1}; StartTime {2}; SHA256 {3}" -f $process.Id, $matches[0].ExecutablePath, $process.StartTime.ToString("o"), $actualHash)
      return
    }
  } while ((Get-Date) -lt $deadline)
  if ($started.HasExited) { throw ("New StudyGuardian.exe exited with code {0}" -f $started.ExitCode) }
  throw "New StudyGuardian.exe did not appear at the verified runtime path"
}

try {
  if ([IO.Path]::GetFullPath($runtimeExe) -ne $expectedRuntimeExe) {
    throw "Runtime path must be exactly $expectedRuntimeExe"
  }
  if (-not (Test-Path -LiteralPath $sourcePath -PathType Container)) {
    throw "WSL source path does not exist: $sourcePath"
  }
  if (-not (Test-Path -LiteralPath $RuntimeDir -PathType Container)) {
    throw "Runtime directory does not exist: $RuntimeDir"
  }
  if (Test-Path -LiteralPath $buildDir) {
    throw "Refusing to reuse build directory: $buildDir"
  }
  New-Item -ItemType Directory -Path $buildDir -Force | Out-Null
  Write-Host "Syncing WSL source to independent build directory: $buildDir"
  & robocopy $sourcePath $buildDir /E /R:1 /W:1 /XJ /XD .git node_modules target dist
  if ($LASTEXITCODE -ge 8) { throw ("robocopy failed with exit code {0}" -f $LASTEXITCODE) }
  if (Test-Path -LiteralPath (Join-Path $buildDir "node_modules")) { throw "Source sync unexpectedly included node_modules" }
  if (-not (Test-Path -LiteralPath $assetSourcePath -PathType Container)) { throw "Referenced pet assets path does not exist: $assetSourcePath" }
  Write-Host "Syncing referenced pet assets required by src/skin.test.ts"
  & robocopy $assetSourcePath $assetBuildPath /E /R:1 /W:1 /XJ /XD .git node_modules target dist __pycache__
  if ($LASTEXITCODE -ge 8) { throw ("robocopy pet assets failed with exit code {0}" -f $LASTEXITCODE) }
  if (-not (Test-Path -LiteralPath (Join-Path $assetBuildPath "skins\studyguardian-pixel\manifest.json") -PathType Leaf)) { throw "Referenced skin manifest was not synchronized" }

  if (Test-Path -LiteralPath (Join-Path $buildDir "src-tauri\target")) { throw "Source sync unexpectedly included target" }

  Invoke-Checked -FilePath "npm.cmd" -Arguments @("ci") -WorkingDirectory $buildDir -Label "npm ci"
  Invoke-Checked -FilePath "npm.cmd" -Arguments @("test") -WorkingDirectory $buildDir -Label "npm test"
  Invoke-Checked -FilePath "npm.cmd" -Arguments @("run", "build") -WorkingDirectory $buildDir -Label "npm run build"
  Invoke-Checked -FilePath "cargo.exe" -Arguments @("check", "--locked", "--manifest-path", (Join-Path $buildDir "src-tauri\Cargo.toml"), "--target-dir", $targetDir) -WorkingDirectory $buildDir -Label "cargo check --locked"
  Invoke-Checked -FilePath "cargo.exe" -Arguments @("test", "--locked", "--manifest-path", (Join-Path $buildDir "src-tauri\Cargo.toml"), "--target-dir", $targetDir) -WorkingDirectory $buildDir -Label "cargo test --locked"
  Invoke-Checked -FilePath "npm.cmd" -Arguments @("run", "tauri", "build", "--", "--debug", "--no-bundle") -WorkingDirectory $buildDir -Label "npm run tauri build -- --debug --no-bundle"
  if (-not (Test-Path -LiteralPath $builtExe -PathType Leaf)) { throw "Tauri debug executable not found: $builtExe" }
  $newHash = (Get-FileHash -LiteralPath $builtExe -Algorithm SHA256).Hash.ToUpperInvariant()
  Write-Host ("Built executable: {0}" -f $builtExe)
  Write-Host ("Built SHA256: {0}" -f $newHash)

  if ($BuildOnly -or $WhatIfPreference) {
    Write-Host "Build complete; runtime replacement skipped (BuildOnly/WhatIf)."
    return
  }

  if (-not (Test-Path -LiteralPath $runtimeExe -PathType Leaf)) { throw "Runtime executable does not exist: $runtimeExe" }
  $oldHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash.ToUpperInvariant()
  Write-Host ("Existing runtime SHA256: {0}" -f $oldHash)
  if (-not $PSCmdlet.ShouldProcess($expectedRuntimeExe, "stop, backup, replace, and restart")) { return }

  Stop-ExactRuntime
  $rollbackRequired = $true
  Copy-Item -LiteralPath $runtimeExe -Destination $backupPath -Force
  if ((Get-FileHash -LiteralPath $backupPath -Algorithm SHA256).Hash.ToUpperInvariant() -ne $oldHash) { throw "Old executable backup hash mismatch" }
  Copy-Item -LiteralPath $builtExe -Destination $temporaryExe -Force
  if ((Get-FileHash -LiteralPath $temporaryExe -Algorithm SHA256).Hash.ToUpperInvariant() -ne $newHash) { throw "Temporary executable hash mismatch" }
  Move-Item -LiteralPath $temporaryExe -Destination $runtimeExe -Force
  if ((Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash.ToUpperInvariant() -ne $newHash) { throw "Runtime executable hash mismatch after replacement" }
  Start-And-VerifyRuntime
  $rollbackRequired = $false
  Write-Host ("Deployment succeeded. Backup: {0}" -f $backupPath)
  Write-Host ("Old SHA256: {0}" -f $oldHash)
  Write-Host ("New SHA256: {0}" -f $newHash)
} catch {
  $failure = $_
  Write-Error ("Deployment failed: " + $failure.Exception.Message)
  if ($temporaryExe -and (Test-Path -LiteralPath $temporaryExe)) { Remove-Item -LiteralPath $temporaryExe -Force -ErrorAction SilentlyContinue }
  if ($rollbackRequired -and $backupPath -and (Test-Path -LiteralPath $backupPath)) {
    try {
      $current = @(Get-RuntimeProcesses)
      foreach ($process in $current) { Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue }
      Wait-ForRuntimeExit
      Wait-ForExclusiveFile -Path $runtimeExe
      Copy-Item -LiteralPath $backupPath -Destination $runtimeExe -Force
      $restoredHash = (Get-FileHash -LiteralPath $runtimeExe -Algorithm SHA256).Hash.ToUpperInvariant()
      if ($restoredHash -ne $oldHash) { throw "Rollback hash mismatch: $restoredHash vs $oldHash" }
      Start-Process -FilePath $runtimeExe -WorkingDirectory $RuntimeDir | Out-Null
      Write-Host "Rollback restored the previous executable and restarted StudyGuardian."
    } catch {
      Write-Error ("Rollback also failed: " + $_.Exception.Message)
    }
  }
  throw
}
