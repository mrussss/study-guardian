param(
    [string]$RootDir,
    [ValidateSet("pyqt", "tauri")][string]$PetRuntime,
    [switch]$NoWatchdog
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
$runtime = if ($PetRuntime) { $PetRuntime } else { Get-StudyGuardianPetRuntime -RootDir $RootDir }
$configDir = Join-Path $RootDir "config"
Write-StudyGuardianLog -RootDir $RootDir -Source "start" -Message "starting runtime; pet=$runtime"

$awExe = Join-Path $RootDir "ActivityWatch\aw-qt.exe"
if (-not (Get-Process -Name "aw-qt", "aw-server" -ErrorAction SilentlyContinue) -and (Test-Path -LiteralPath $awExe)) {
    Start-Process -FilePath $awExe -WorkingDirectory (Split-Path -Parent $awExe)
}

$sensorScript = Join-Path $RootDir "sensor\screen\server.py"
if (-not (Test-StudyGuardianPort -Port 17322)) {
    $sensorPython = Join-Path $RootDir "sensor\.venv\Scripts\python.exe"
    if (-not (Test-Path -LiteralPath $sensorPython)) { $sensorPython = (Get-Command python.exe).Source }
    Start-Process -FilePath $sensorPython -ArgumentList @((ConvertTo-StudyGuardianArgument $sensorScript), "--token-file", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "auth.token"))) -WorkingDirectory (Split-Path -Parent $sensorScript) -WindowStyle Hidden
}

$supervisorExe = Join-Path $RootDir "bin\study-supervisor.exe"
if (-not (Test-StudyGuardianPort -Port 17321)) {
    if (-not (Test-Path -LiteralPath $supervisorExe)) { throw "Supervisor executable not found: $supervisorExe" }
    Start-Process -FilePath $supervisorExe -ArgumentList @(
        "-config", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "config.yaml")), "-token", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "auth.token")),
        "-collector-token", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "collector-token")), "-db", (ConvertTo-StudyGuardianArgument (Join-Path $RootDir "data\studyguardian.db"))
    ) -WorkingDirectory $RootDir -WindowStyle Hidden
}
if (-not (Wait-StudyGuardianPort -Port 17321 -TimeoutSeconds 10)) { throw "Supervisor did not become healthy on port 17321" }

$legacyPet = Join-Path $RootDir "pet\src\main.py"
$tauriPet = Join-Path $RootDir "pet-v3\StudyGuardian.exe"
if ($runtime -eq "tauri") {
    Stop-OwnedPythonScript -ScriptPath $legacyPet
    if (-not (Test-Path -LiteralPath $tauriPet)) { throw "Tauri Pet executable not found: $tauriPet" }
    if (-not (Test-OwnedExecutable -Path $tauriPet)) { Start-Process -FilePath $tauriPet -WorkingDirectory (Split-Path -Parent $tauriPet) }
} else {
    if (-not (Test-Path -LiteralPath $legacyPet)) { throw "Legacy Pet script not found: $legacyPet" }
    if (-not (Test-OwnedPythonScript -ScriptPath $legacyPet)) {
        $petPython = Join-Path $RootDir "pet\.venv\Scripts\python.exe"
        if (-not (Test-Path -LiteralPath $petPython)) { $petPython = (Get-Command python.exe).Source }
        Start-Process -FilePath $petPython -ArgumentList @(
            (ConvertTo-StudyGuardianArgument $legacyPet), "--token-file", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "auth.token")),
            "--assets", (ConvertTo-StudyGuardianArgument (Join-Path $RootDir "pet\assets")), "--pet-config", (ConvertTo-StudyGuardianArgument (Join-Path $configDir "pet.json"))
        ) -WorkingDirectory (Split-Path -Parent $legacyPet)
    }
}

if (-not $NoWatchdog) {
    $watchdogScript = Join-Path $RootDir "scripts\watchdog.ps1"
    $watchdogRunning = @(Get-StudyGuardianProcesses | Where-Object {
        $_.ProcessId -ne $PID -and ($_.Name -eq "powershell.exe" -or $_.Name -eq "pwsh.exe") -and $_.CommandLine -and
        $_.CommandLine.IndexOf($watchdogScript, [StringComparison]::OrdinalIgnoreCase) -ge 0
    }).Count -gt 0
    if (-not $watchdogRunning) {
        Start-Process -FilePath (Get-Command powershell.exe).Source -ArgumentList @(
            "-NoProfile", "-WindowStyle", "Hidden", "-ExecutionPolicy", "Bypass", "-File", (ConvertTo-StudyGuardianArgument $watchdogScript),
            "-RootDir", (ConvertTo-StudyGuardianArgument $RootDir)
        ) -WorkingDirectory $RootDir -WindowStyle Hidden
    }
}
Write-StudyGuardianLog -RootDir $RootDir -Source "start" -Message "runtime started; pet=$runtime"
