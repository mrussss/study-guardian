[CmdletBinding(SupportsShouldProcess)]
param(
    [ValidateSet("Status", "Prune", "Reset")]
    [string]$Action = "Status",
    [string]$BuildRoot = "D:\StudyGuardianBuild",
    [switch]$TestMode
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$productionRoot = [IO.Path]::GetFullPath("D:\StudyGuardianBuild")
$testRoot = [IO.Path]::GetFullPath("D:\StudyGuardianBuild\test-cache")
$BuildRoot = [IO.Path]::GetFullPath($BuildRoot)

if ($TestMode) {
    if (-not $BuildRoot.StartsWith("$testRoot\", [StringComparison]::OrdinalIgnoreCase)) {
        throw "Test cache paths must remain under $testRoot"
    }
} elseif ($BuildRoot -ne $productionRoot) {
    throw "Production cache root must be exactly $productionRoot"
}

function Get-TreeMetrics {
    param([Parameter(Mandatory)] [string]$Path)
    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        return [pscustomobject]@{ Path = $Path; Files = 0; SizeGB = 0 }
    }
    $measure = Get-ChildItem -LiteralPath $Path -Recurse -File -Force -ErrorAction SilentlyContinue | Measure-Object Length -Sum
    return [pscustomobject]@{ Path = $Path; Files = $measure.Count; SizeGB = [math]::Round(($measure.Sum / 1GB), 2) }
}

function Assert-SafeChild {
    param([Parameter(Mandatory)] [string]$Path)
    $full = [IO.Path]::GetFullPath($Path)
    if (-not $full.StartsWith("$BuildRoot\", [StringComparison]::OrdinalIgnoreCase)) {
        throw "Cache operation escaped its root: $full"
    }
    return $full
}

function Remove-SafeDirectory {
    param([Parameter(Mandatory)] [string]$Path)
    $full = Assert-SafeChild -Path $Path
    if (Test-Path -LiteralPath $full -PathType Container) {
        [IO.Directory]::Delete($full, $true)
    }
}

function Show-Status {
    $rows = @(
        Get-TreeMetrics -Path (Join-Path $BuildRoot "artifacts")
        Get-TreeMetrics -Path (Join-Path $BuildRoot "backups")
        Get-TreeMetrics -Path (Join-Path $BuildRoot "cargo-target")
        Get-TreeMetrics -Path (Join-Path $BuildRoot "npm-cache")
        Get-TreeMetrics -Path (Join-Path $BuildRoot "staging")
        Get-TreeMetrics -Path (Join-Path $BuildRoot "tools")
    )
    $rows | Format-Table -AutoSize
    $total = Get-TreeMetrics -Path $BuildRoot
    Write-Host ("Total: {0} files, {1} GB at {2}" -f $total.Files, $total.SizeGB, $BuildRoot)
}

if ($Action -eq "Status") { Show-Status; return }

if ($Action -eq "Prune") {
    if (-not (Test-Path -LiteralPath $BuildRoot -PathType Container)) { Write-Host "Build cache is empty."; return }
    $before = Get-TreeMetrics -Path $BuildRoot
    if ($PSCmdlet.ShouldProcess($BuildRoot, "prune old Pet backups, logs, temporary test data, and stale release cache")) {
        $backupRoot = Assert-SafeChild -Path (Join-Path $BuildRoot "backups\pet-v3")
        if (Test-Path -LiteralPath $backupRoot) {
            $backups = @(Get-ChildItem -LiteralPath $backupRoot -Filter "StudyGuardian-*.exe" -File | Sort-Object LastWriteTime -Descending)
            foreach ($backup in ($backups | Select-Object -Skip 3)) { Remove-Item -LiteralPath $backup.FullName -Force }
        }
        $logsRoot = Assert-SafeChild -Path (Join-Path $BuildRoot "logs")
        if (Test-Path -LiteralPath $logsRoot) {
            Get-ChildItem -LiteralPath $logsRoot -File -Force | Where-Object LastWriteTime -lt (Get-Date).AddDays(-14) | Remove-Item -Force
        }
        $testRuntime = Assert-SafeChild -Path (Join-Path $BuildRoot "test-runtime")
        if (Test-Path -LiteralPath $testRuntime) {
            Get-ChildItem -LiteralPath $testRuntime -Directory -Force | Where-Object LastWriteTime -lt (Get-Date).AddDays(-1) | ForEach-Object { Remove-SafeDirectory -Path $_.FullName }
        }
        $releaseCache = Assert-SafeChild -Path (Join-Path $BuildRoot "cargo-target\release-cache")
        if (Test-Path -LiteralPath $releaseCache) {
            $newest = Get-ChildItem -LiteralPath $releaseCache -Recurse -File -Force -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending | Select-Object -First 1
            if ($newest -and $newest.LastWriteTime -lt (Get-Date).AddDays(-30)) { Remove-SafeDirectory -Path $releaseCache }
        }
    }
    $after = Get-TreeMetrics -Path $BuildRoot
    Write-Host ("Pruned {0} GB; cache now uses {1} GB." -f [math]::Round(($before.SizeGB - $after.SizeGB), 2), $after.SizeGB)
    return
}

$active = @(Get-CimInstance Win32_Process | Where-Object {
    $_.ExecutablePath -and [IO.Path]::GetFullPath($_.ExecutablePath).StartsWith("$BuildRoot\", [StringComparison]::OrdinalIgnoreCase)
})
if ($active.Count -gt 0) { throw "Refusing cache reset while build-cache tools are running: $($active.ProcessId -join ', ')" }
if ($PSCmdlet.ShouldProcess($BuildRoot, "delete the complete rebuildable Pet build cache")) {
    if (Test-Path -LiteralPath $BuildRoot -PathType Container) {
        if ($TestMode) { [IO.Directory]::Delete($BuildRoot, $true) }
        else {
            Get-ChildItem -LiteralPath $BuildRoot -Force | ForEach-Object {
                $full = Assert-SafeChild -Path $_.FullName
                if ($_.PSIsContainer) { [IO.Directory]::Delete($full, $true) } else { Remove-Item -LiteralPath $full -Force }
            }
        }
    }
    Write-Host "Reset complete. The next native build will recreate $BuildRoot."
}
