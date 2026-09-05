param(
    [string]$RootDir,
    [int]$IntervalSeconds = 5,
    [int]$RetryWindowMinutes = 5,
    [int]$MaxRetries = 3
)

$ErrorActionPreference = "SilentlyContinue"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
$IntervalSeconds = [Math]::Max(2, $IntervalSeconds)
$RetryWindowMinutes = [Math]::Max(1, $RetryWindowMinutes)
$MaxRetries = [Math]::Max(1, $MaxRetries)
$startScript = Join-Path $RootDir "scripts\start-all.ps1"

function Test-StudyGuardianHealthy {
    $runtime = Get-StudyGuardianPetRuntime -RootDir $RootDir
    $petHealthy = if ($runtime -eq "tauri") {
        Test-OwnedExecutable -Path (Join-Path $RootDir "pet-v3\StudyGuardian.exe")
    } else {
        Test-OwnedPythonScript -ScriptPath (Join-Path $RootDir "pet\src\main.py")
    }
    return (Test-StudyGuardianPort -Port 17321) -and (Test-StudyGuardianPort -Port 17322) -and $petHealthy
}

$retryTimes = New-Object 'System.Collections.Generic.Queue[datetime]'
Write-StudyGuardianLog -RootDir $RootDir -Source "watchdog" -Message "started"
while ($true) {
    Start-Sleep -Seconds $IntervalSeconds
    if (Test-StudyGuardianHealthy) { continue }
    $now = Get-Date
    while ($retryTimes.Count -gt 0 -and ($now - $retryTimes.Peek()).TotalMinutes -ge $RetryWindowMinutes) { [void]$retryTimes.Dequeue() }
    if ($retryTimes.Count -ge $MaxRetries) {
        Write-StudyGuardianLog -RootDir $RootDir -Source "watchdog" -Message "retry window exhausted"
        Start-Sleep -Seconds ([Math]::Max(30, $RetryWindowMinutes * 60))
        continue
    }
    [void]$retryTimes.Enqueue($now)
    $backoff = [Math]::Min(60, [int]([Math]::Pow(2, $retryTimes.Count - 1) * 5))
    Start-Sleep -Seconds $backoff
    & powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File $startScript -RootDir $RootDir -NoWatchdog | Out-Null
}
