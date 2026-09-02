# Safely rebuild the isolated Pet and Sensor environments.
param(
    [string]$RootDir = "D:\StudyGuardianDev",
    [string]$Python = "python.exe"
)
$ErrorActionPreference = "Stop"

$petDir = Join-Path $RootDir "pet"
$sensorDir = Join-Path $RootDir "sensor"
$petVenv = Join-Path $petDir ".venv"
$sensorVenv = Join-Path $sensorDir ".venv"
$petNew = Join-Path $petDir ".venv.new"
$sensorNew = Join-Path $sensorDir ".venv.new"
$petBackup = Join-Path $petDir ".venv.backup"
$sensorBackup = Join-Path $sensorDir ".venv.backup"
$petSwapped = $false
$sensorSwapped = $false

foreach ($path in @((Join-Path $petDir "requirements.txt"), (Join-Path $sensorDir "requirements.txt"))) {
    if (-not (Test-Path -LiteralPath $path)) { throw "Requirements file not found: $path" }
}
foreach ($backup in @($petBackup, $sensorBackup)) {
    if (Test-Path -LiteralPath $backup) { throw "Backup already exists; inspect before retrying: $backup" }
}

$stopScript = Join-Path $RootDir "scripts\stop-all.ps1"
if (Test-Path -LiteralPath $stopScript) { & $stopScript }

try {
    foreach ($newVenv in @($petNew, $sensorNew)) {
        if (Test-Path -LiteralPath $newVenv) { Remove-Item -LiteralPath $newVenv -Recurse -Force }
        & $Python -m venv $newVenv
        if ($LASTEXITCODE -ne 0) { throw "Could not create $newVenv" }
    }

    & (Join-Path $petNew "Scripts\python.exe") -m pip install --upgrade pip
    & (Join-Path $petNew "Scripts\python.exe") -m pip install -r (Join-Path $petDir "requirements.txt")
    & (Join-Path $petNew "Scripts\python.exe") -m pip check
    if ($LASTEXITCODE -ne 0) { throw "Pet dependency check failed" }

    & (Join-Path $sensorNew "Scripts\python.exe") -m pip install --upgrade pip
    & (Join-Path $sensorNew "Scripts\python.exe") -m pip install -r (Join-Path $sensorDir "requirements.txt")
    & (Join-Path $sensorNew "Scripts\python.exe") -m pip check
    if ($LASTEXITCODE -ne 0) { throw "Sensor dependency check failed" }

    if (Test-Path -LiteralPath $petVenv) { Move-Item -LiteralPath $petVenv -Destination $petBackup }
    if (Test-Path -LiteralPath $sensorVenv) { Move-Item -LiteralPath $sensorVenv -Destination $sensorBackup }
    Move-Item -LiteralPath $petNew -Destination $petVenv
    $petSwapped = $true
    Move-Item -LiteralPath $sensorNew -Destination $sensorVenv
    $sensorSwapped = $true

    & (Join-Path $petVenv "Scripts\python.exe") -c "import PyQt6"
    if ($LASTEXITCODE -ne 0) { throw "Pet PyQt6 smoke failed" }
    & (Join-Path $sensorVenv "Scripts\python.exe") -c "import mss; from PIL import Image"
    if ($LASTEXITCODE -ne 0) { throw "Sensor capture dependency smoke failed" }

    Remove-Item -LiteralPath $petBackup, $sensorBackup -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "Python environments rebuilt and smoke-checked successfully." -ForegroundColor Green
}
catch {
    foreach ($newVenv in @($petNew, $sensorNew)) {
        if (Test-Path -LiteralPath $newVenv) { Remove-Item -LiteralPath $newVenv -Recurse -Force }
    }
    if ($petSwapped -and (Test-Path -LiteralPath $petVenv)) {
        Remove-Item -LiteralPath $petVenv -Recurse -Force
    }
    if ($sensorSwapped -and (Test-Path -LiteralPath $sensorVenv)) {
        Remove-Item -LiteralPath $sensorVenv -Recurse -Force
    }
    if ((Test-Path -LiteralPath $petBackup) -and -not (Test-Path -LiteralPath $petVenv)) {
        Move-Item -LiteralPath $petBackup -Destination $petVenv
    }
    if ((Test-Path -LiteralPath $sensorBackup) -and -not (Test-Path -LiteralPath $sensorVenv)) {
        Move-Item -LiteralPath $sensorBackup -Destination $sensorVenv
    }
    throw
}
