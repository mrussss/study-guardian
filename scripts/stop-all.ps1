param([string]$RootDir)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
$watchdogScript = Join-Path $RootDir "scripts\watchdog.ps1"
Get-StudyGuardianProcesses | Where-Object {
    $_.ProcessId -ne $PID -and ($_.Name -eq "powershell.exe" -or $_.Name -eq "pwsh.exe") -and $_.CommandLine -and
    $_.CommandLine.IndexOf($watchdogScript, [StringComparison]::OrdinalIgnoreCase) -ge 0
} | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }

Stop-OwnedExecutable -Path (Join-Path $RootDir "bin\study-supervisor.exe")
Stop-OwnedExecutable -Path (Join-Path $RootDir "pet-v3\StudyGuardian.exe")
Stop-OwnedPythonScript -ScriptPath (Join-Path $RootDir "sensor\screen\server.py")
Stop-OwnedPythonScript -ScriptPath (Join-Path $RootDir "pet\src\main.py")
Write-StudyGuardianLog -RootDir $RootDir -Source "stop" -Message "owned runtime processes stopped"
