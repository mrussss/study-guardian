Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$script = Join-Path $repoRoot "scripts\manage-pet-v3-cache.ps1"
$testRoot = [IO.Path]::GetFullPath((Join-Path "D:\StudyGuardianBuild\test-cache" ([guid]::NewGuid().ToString("N"))))

try {
    $backupRoot = Join-Path $testRoot "backups\pet-v3"
    $logsRoot = Join-Path $testRoot "logs"
    New-Item -ItemType Directory -Path $backupRoot, $logsRoot -Force | Out-Null
    1..5 | ForEach-Object {
        $path = Join-Path $backupRoot ("StudyGuardian-2026090{0}.exe" -f $_)
        [IO.File]::WriteAllText($path, "backup-$_")
        (Get-Item -LiteralPath $path).LastWriteTime = (Get-Date).AddMinutes($_)
    }
    $oldLog = Join-Path $logsRoot "old.log"
    [IO.File]::WriteAllText($oldLog, "old")
    (Get-Item -LiteralPath $oldLog).LastWriteTime = (Get-Date).AddDays(-20)

    & $script -Action Prune -BuildRoot $testRoot -TestMode
    if (@(Get-ChildItem -LiteralPath $backupRoot -Filter "StudyGuardian-*.exe" -File).Count -ne 3) { throw "Prune did not retain exactly three backups" }
    if (Test-Path -LiteralPath $oldLog) { throw "Prune did not remove an expired log" }

    $guarded = $false
    try { & $script -Action Reset -BuildRoot "C:\StudyGuardianBuild" -TestMode } catch { $guarded = $true }
    if (-not $guarded) { throw "Cache reset accepted a path outside the D: test root" }

    & $script -Action Reset -BuildRoot $testRoot -TestMode -Confirm:$false
    if (Test-Path -LiteralPath $testRoot) { throw "Cache reset left the test root behind" }
    Write-Host "PASS: Pet v3 cache prune, reset, and path guards"
} finally {
    if (Test-Path -LiteralPath $testRoot) { [IO.Directory]::Delete($testRoot, $true) }
}
