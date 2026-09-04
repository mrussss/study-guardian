# Lightweight child-process crash recovery for the Windows runtime.
# It intentionally monitors only StudyGuardian-owned Supervisor, Sensor and
# Pet processes. User data and configuration are never read or modified.
param(
    [string]$RootDir = "D:\StudyGuardianDev",
    [int]$IntervalSeconds = 5,
    [int]$RetryWindowMinutes = 5,
    [int]$MaxRetries = 3
)

$ErrorActionPreference = "SilentlyContinue"
$IntervalSeconds = [Math]::Max(2, $IntervalSeconds)
$RetryWindowMinutes = [Math]::Max(1, $RetryWindowMinutes)
$MaxRetries = [Math]::Max(1, $MaxRetries)
$LogPath = Join-Path $RootDir "logs\watchdog.log"
$StartScript = Join-Path $RootDir "scripts\start-all.ps1"

function Write-WatchdogLog([string]$Message) {
    $line = "{0:u} [Watchdog] {1}" -f (Get-Date), $Message
    try {
        New-Item -ItemType Directory -Path (Split-Path -Parent $LogPath) -Force | Out-Null
        Add-Content -LiteralPath $LogPath -Value $line -Encoding UTF8
    } catch {
        # Logging must never prevent recovery or leak an exception to the UI.
    }
}

function Test-ListeningPort([int]$Port) {
    return @(Get-NetTCPConnection -State Listen -LocalPort $Port).Count -gt 0
}

function Test-PetProcess {
    return @(Get-CimInstance Win32_Process | Where-Object {
        $_.Name -eq "python.exe" -and $_.CommandLine -like "*$RootDir\pet\src\main.py*"
    }).Count -gt 0
}

function Test-StudyGuardianHealthy {
    $supervisor = @(Get-Process -Name "study-supervisor").Count -gt 0 -and (Test-ListeningPort 17321)
    $sensor = Test-ListeningPort 17322
    $pet = Test-PetProcess
    return $supervisor -and $sensor -and $pet
}

function Invoke-Recovery {
    if (-not (Test-Path -LiteralPath $StartScript)) {
        Write-WatchdogLog "recovery skipped: start script unavailable"
        return
    }
    Write-WatchdogLog "recovery attempt: starting missing runtime component(s)"
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $StartScript -NoWatchdog | Out-Null
}

$retryTimes = New-Object 'System.Collections.Generic.Queue[datetime]'
Write-WatchdogLog "started; interval=${IntervalSeconds}s max_retries=${MaxRetries} window=${RetryWindowMinutes}m"

while ($true) {
    Start-Sleep -Seconds $IntervalSeconds
    if (Test-StudyGuardianHealthy) {
        while ($retryTimes.Count -gt 0 -and ((Get-Date) - $retryTimes.Peek()).TotalMinutes -ge $RetryWindowMinutes) {
            [void]$retryTimes.Dequeue()
        }
        continue
    }

    $now = Get-Date
    while ($retryTimes.Count -gt 0 -and ($now - $retryTimes.Peek()).TotalMinutes -ge $RetryWindowMinutes) {
        [void]$retryTimes.Dequeue()
    }
    if ($retryTimes.Count -ge $MaxRetries) {
        Write-WatchdogLog "retry window exhausted; waiting ${RetryWindowMinutes}m before another recovery window"
        Start-Sleep -Seconds ([Math]::Max(30, $RetryWindowMinutes * 60))
        continue
    }

    $attempt = $retryTimes.Count + 1
    [void]$retryTimes.Enqueue($now)
    $backoff = [Math]::Min(60, [int]([Math]::Pow(2, $attempt - 1) * 5))
    Write-WatchdogLog "component unavailable; retry ${attempt}/${MaxRetries} after ${backoff}s backoff"
    Start-Sleep -Seconds $backoff
    Invoke-Recovery
}
