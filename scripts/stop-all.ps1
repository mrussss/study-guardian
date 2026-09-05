param([string]$RootDir)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
$watchdogScript = Join-Path $RootDir "scripts\watchdog.ps1"
Stop-OwnedPowerShellScript -ScriptPath $watchdogScript

Stop-OwnedExecutable -Path (Join-Path $RootDir "bin\study-supervisor.exe")
Stop-OwnedExecutable -Path (Join-Path $RootDir "pet-v3\StudyGuardian.exe")
Stop-OwnedPythonScript -ScriptPath (Join-Path $RootDir "sensor\screen\server.py")
Stop-OwnedPythonScript -ScriptPath (Join-Path $RootDir "pet\src\main.py")
Write-StudyGuardianLog -RootDir $RootDir -Source "stop" -Message "owned runtime processes stopped"
