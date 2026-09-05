$ErrorActionPreference = "Stop"

function Resolve-StudyGuardianRoot {
    param([string]$RootDir, [string]$CallingScriptRoot = $PSScriptRoot)
    $candidate = if ([string]::IsNullOrWhiteSpace($RootDir)) { Split-Path -Parent $CallingScriptRoot } else { $RootDir }
    return [IO.Path]::GetFullPath($candidate).TrimEnd('\')
}

function Get-StudyGuardianPetRuntime {
    param([Parameter(Mandatory = $true)][string]$RootDir)
    $path = Join-Path $RootDir "config\runtime.json"
    if (Test-Path -LiteralPath $path) {
        try {
            $value = Get-Content -LiteralPath $path -Raw -Encoding UTF8 | ConvertFrom-Json
            if ($value.pet_runtime -eq "tauri") { return "tauri" }
        } catch {}
    }
    return "pyqt"
}

function ConvertTo-StudyGuardianArgument {
    param([Parameter(Mandatory = $true)][string]$Value)
    if ($Value.Contains('"')) { throw "Process argument contains an unsupported quote" }
    return "`"$Value`""
}

function Test-StudyGuardianPort {
    param([Parameter(Mandatory = $true)][int]$Port)
    return @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue).Count -gt 0
}

function Get-StudyGuardianProcesses {
    try { return @(Get-CimInstance Win32_Process -ErrorAction Stop) } catch { return @() }
}

function Test-OwnedExecutable {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path -LiteralPath $Path)) { return $false }
    $expected = [IO.Path]::GetFullPath($Path)
    return @(Get-StudyGuardianProcesses | Where-Object {
        $_.ExecutablePath -and [string]::Equals([IO.Path]::GetFullPath($_.ExecutablePath), $expected, [StringComparison]::OrdinalIgnoreCase)
    }).Count -gt 0
}

function Test-OwnedPythonScript {
    param([Parameter(Mandatory = $true)][string]$ScriptPath)
    $expected = [IO.Path]::GetFullPath($ScriptPath)
    return @(Get-StudyGuardianProcesses | Where-Object {
        ($_.Name -eq "python.exe" -or $_.Name -eq "pythonw.exe") -and $_.CommandLine -and
        $_.CommandLine.IndexOf($expected, [StringComparison]::OrdinalIgnoreCase) -ge 0
    }).Count -gt 0
}

function Stop-OwnedExecutable {
    param([Parameter(Mandatory = $true)][string]$Path)
    $expected = [IO.Path]::GetFullPath($Path)
    Get-StudyGuardianProcesses | Where-Object {
        $_.ProcessId -ne $PID -and $_.ExecutablePath -and
        [string]::Equals([IO.Path]::GetFullPath($_.ExecutablePath), $expected, [StringComparison]::OrdinalIgnoreCase)
    } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Stop-OwnedPythonScript {
    param([Parameter(Mandatory = $true)][string]$ScriptPath)
    $expected = [IO.Path]::GetFullPath($ScriptPath)
    Get-StudyGuardianProcesses | Where-Object {
        $_.ProcessId -ne $PID -and ($_.Name -eq "python.exe" -or $_.Name -eq "pythonw.exe") -and $_.CommandLine -and
        $_.CommandLine.IndexOf($expected, [StringComparison]::OrdinalIgnoreCase) -ge 0
    } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
}

function Wait-StudyGuardianPort {
    param([Parameter(Mandatory = $true)][int]$Port, [int]$TimeoutSeconds = 10)
    $deadline = (Get-Date).AddSeconds([Math]::Max(1, $TimeoutSeconds))
    do {
        if (Test-StudyGuardianPort -Port $Port) { return $true }
        Start-Sleep -Milliseconds 250
    } while ((Get-Date) -lt $deadline)
    return $false
}

function Write-StudyGuardianLog {
    param([Parameter(Mandatory = $true)][string]$RootDir, [Parameter(Mandatory = $true)][string]$Source, [Parameter(Mandatory = $true)][string]$Message)
    try {
        $logDir = Join-Path $RootDir "logs"
        $logPath = Join-Path $logDir "launcher.log"
        New-Item -ItemType Directory -Path $logDir -Force | Out-Null
        if ((Test-Path -LiteralPath $logPath) -and (Get-Item -LiteralPath $logPath).Length -gt 1048576) {
            Move-Item -LiteralPath $logPath -Destination "$logPath.previous" -Force
        }
        Add-Content -LiteralPath $logPath -Encoding UTF8 -Value ("{0:u} [{1}] {2}" -f (Get-Date), $Source, $Message)
    } catch {}
}
