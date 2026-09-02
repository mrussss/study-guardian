# StudyGuardian 一键启动脚本 (Windows PowerShell)
$ErrorActionPreference = "Stop"

$RootDir = "D:\StudyGuardianDev"
$BinDir = "$RootDir\bin"
$PetDir = "$RootDir\pet"
$SensorDir = "$RootDir\sensor"
$ConfigDir = "$RootDir\config"
$LogsDir = "$RootDir\logs"

Write-Host "==================================================" -ForegroundColor Cyan
Write-Host "  Starting StudyGuardian Components" -ForegroundColor Cyan
Write-Host "==================================================" -ForegroundColor Cyan

# 1. Start ActivityWatch if not running
$awProcess = Get-Process -Name "aw-qt", "aw-server" -ErrorAction SilentlyContinue
if (-not $awProcess) {
    Write-Host "[1/4] Starting ActivityWatch..." -ForegroundColor Yellow
    $awExe = "$RootDir\ActivityWatch\aw-qt.exe"
    if (Test-Path $awExe) {
        Start-Process -FilePath $awExe -WorkingDirectory "$RootDir\ActivityWatch"
    } else {
        Write-Host "Warning: ActivityWatch not found at $awExe, please start it manually." -ForegroundColor DarkYellow
    }
} else {
    Write-Host "[1/4] ActivityWatch is already running." -ForegroundColor Green
}

# 2. Start Screen Sensor
Write-Host "[2/4] Starting Screen Sensor (:17322)..." -ForegroundColor Yellow
$sensorVenvPy = "$SensorDir\.venv\Scripts\python.exe"
if (-not (Test-Path $sensorVenvPy)) {
    $sensorVenvPy = "python.exe"
}
$sensorScript = "$SensorDir\screen\server.py"
$sensorTokenFile = "$ConfigDir\auth.token"
Start-Process -FilePath $sensorVenvPy -ArgumentList "`"$sensorScript`" --token-file `"$sensorTokenFile`"" -WorkingDirectory "$SensorDir\screen" -WindowStyle Hidden

# 3. Start Supervisor
Write-Host "[3/4] Starting Supervisor (:17321)..." -ForegroundColor Yellow
$supExe = "$BinDir\study-supervisor.exe"
$supConfig = "$ConfigDir\config.yaml"
$supToken = "$ConfigDir\auth.token"
$supDB = "$RootDir\data\studyguardian.db"
Start-Process -FilePath $supExe -ArgumentList "-config `"$supConfig`" -token `"$supToken`" -db `"$supDB`"" -WorkingDirectory "$RootDir" -WindowStyle Hidden

Start-Sleep -Seconds 2

# 4. Start Pet UI Shell
Write-Host "[4/4] Starting Study Pet UI..." -ForegroundColor Yellow
$petVenvPy = "$PetDir\.venv\Scripts\python.exe"
if (-not (Test-Path $petVenvPy)) {
    $petVenvPy = "python.exe"
}
$petScript = "$PetDir\src\main.py"
$petAssets = "$PetDir\assets"
Start-Process -FilePath $petVenvPy -ArgumentList "`"$petScript`" --token-file `"$supToken`" --assets `"$petAssets`"" -WorkingDirectory "$PetDir\src"

Write-Host "==================================================" -ForegroundColor Green
Write-Host "  StudyGuardian is now running in background!" -ForegroundColor Green
Write-Host "==================================================" -ForegroundColor Green
