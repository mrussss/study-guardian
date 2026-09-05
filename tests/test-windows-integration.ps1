$ErrorActionPreference = "Stop"
$repoRoot = Split-Path -Parent $PSScriptRoot
. (Join-Path $repoRoot "scripts\runtime-common.ps1")

function Assert-True([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
$tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("StudyGuardian Script Test " + [Guid]::NewGuid().ToString("N"))
$startup = Join-Path $tempRoot "Startup Folder"
$desktop = Join-Path $tempRoot "Desktop Folder"
try {
    New-Item -ItemType Directory -Path (Join-Path $tempRoot "scripts"), (Join-Path $tempRoot "config"), $startup, $desktop -Force | Out-Null
    Set-Content -LiteralPath (Join-Path $tempRoot "scripts\launch-studyguardian.ps1") -Value "param()" -Encoding UTF8
    $resolved = Resolve-StudyGuardianRoot -RootDir "" -CallingScriptRoot (Join-Path $tempRoot "scripts")
    Assert-True ([string]::Equals($resolved, $tempRoot, [StringComparison]::OrdinalIgnoreCase)) "root derivation failed"
    Assert-True ((Get-StudyGuardianPetRuntime -RootDir $tempRoot) -eq "pyqt") "safe runtime default failed"
    Set-Content -LiteralPath (Join-Path $tempRoot "config\runtime.json") -Value '{"pet_runtime":"tauri"}' -Encoding UTF8
    Assert-True ((Get-StudyGuardianPetRuntime -RootDir $tempRoot) -eq "tauri") "runtime marker failed"
    $watchdogPath = Join-Path $tempRoot "scripts\watchdog.ps1"
    $escapedWatchdog = [Regex]::Escape([IO.Path]::GetFullPath($watchdogPath))
    $ownedPattern = "(?i)(?:^|\s)-File\s+(?:`"$escapedWatchdog`"|$escapedWatchdog)(?:\s|$)"
    Assert-True ([Regex]::IsMatch("powershell.exe -File `"$watchdogPath`" -RootDir `"$tempRoot`"", $ownedPattern)) "watchdog command matching failed"
    Assert-True (-not [Regex]::IsMatch("powershell.exe -File stop-all.ps1 -Note `"$watchdogPath`"", $ownedPattern)) "watchdog command matching is too broad"

    $autostart = Join-Path $repoRoot "scripts\set-autostart.ps1"
    & $autostart -Enable -RootDir $tempRoot -StartupDirectory $startup | Out-Null
    & $autostart -Enable -RootDir $tempRoot -StartupDirectory $startup | Out-Null
    Assert-True (@(Get-ChildItem -LiteralPath $startup -Filter "StudyGuardian.lnk").Count -eq 1) "autostart is not idempotent"
    Assert-True ((& $autostart -GetState -RootDir $tempRoot -StartupDirectory $startup) -eq "enabled") "autostart state failed"
    & $autostart -Disable -RootDir $tempRoot -StartupDirectory $startup | Out-Null
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $startup "StudyGuardian.lnk"))) "autostart removal failed"

    $desktopScript = Join-Path $repoRoot "scripts\set-desktop-shortcut.ps1"
    & $desktopScript -Create -RootDir $tempRoot -DesktopDirectory $desktop | Out-Null
    Assert-True ((& $desktopScript -GetState -RootDir $tempRoot -DesktopDirectory $desktop) -eq "enabled") "desktop shortcut state failed"
    & $desktopScript -Remove -RootDir $tempRoot -DesktopDirectory $desktop | Out-Null
    Assert-True (-not (Test-Path -LiteralPath (Join-Path $desktop "StudyGuardian.lnk"))) "desktop shortcut removal failed"
    Write-Host "Windows integration script tests passed."
} finally {
    if (Test-Path -LiteralPath $tempRoot) { Remove-Item -LiteralPath $tempRoot -Recurse -Force }
}
