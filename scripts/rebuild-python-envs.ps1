# Rebuild the two isolated Python environments used by StudyGuardian.
# This script intentionally removes only the named virtual-environment folders.
param(
    [string]$RootDir = "D:\StudyGuardianDev",
    [string]$Python = "python.exe"
)
$ErrorActionPreference = "Stop"

$petVenv = Join-Path $RootDir "pet\.venv"
$sensorVenv = Join-Path $RootDir "sensor\.venv"
$petReq = Join-Path $RootDir "pet\requirements.txt"
$sensorReq = Join-Path $RootDir "sensor\requirements.txt"

foreach ($path in @($petReq, $sensorReq)) {
    if (-not (Test-Path -LiteralPath $path)) { throw "Requirements file not found: $path" }
}
foreach ($venv in @($petVenv, $sensorVenv)) {
    if (Test-Path -LiteralPath $venv) { Remove-Item -LiteralPath $venv -Recurse -Force }
    & $Python -m venv $venv
    if ($LASTEXITCODE -ne 0) { throw "Could not create venv: $venv" }
}

& (Join-Path $petVenv "Scripts\python.exe") -m pip install --upgrade pip
& (Join-Path $petVenv "Scripts\python.exe") -m pip install -r $petReq
& (Join-Path $sensorVenv "Scripts\python.exe") -m pip install --upgrade pip
& (Join-Path $sensorVenv "Scripts\python.exe") -m pip install -r $sensorReq
if ($LASTEXITCODE -ne 0) { throw "Python dependency installation failed" }
Write-Host "Python environments rebuilt under $RootDir" -ForegroundColor Green
