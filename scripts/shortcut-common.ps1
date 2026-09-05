. (Join-Path $PSScriptRoot "runtime-common.ps1")

function Set-StudyGuardianShortcut {
    param(
        [Parameter(Mandatory = $true)][string]$ShortcutPath,
        [Parameter(Mandatory = $true)][string]$RootDir,
        [Parameter(Mandatory = $true)][string]$LauncherSwitch,
        [Parameter(Mandatory = $true)][string]$Description
    )
    $launcher = Join-Path $RootDir "scripts\launch-studyguardian.ps1"
    if (-not (Test-Path -LiteralPath $launcher)) { throw "Stable launcher not found: $launcher" }
    New-Item -ItemType Directory -Path (Split-Path -Parent $ShortcutPath) -Force | Out-Null
    $shell = New-Object -ComObject WScript.Shell
    $shortcut = $shell.CreateShortcut($ShortcutPath)
    $shortcut.TargetPath = (Get-Command powershell.exe).Source
    $shortcut.Arguments = "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$launcher`" $LauncherSwitch -RootDir `"$RootDir`""
    $shortcut.WorkingDirectory = $RootDir
    $shortcut.Description = $Description
    $icon = Join-Path $RootDir "pet-v3\StudyGuardian.exe"
    if (Test-Path -LiteralPath $icon) { $shortcut.IconLocation = "$icon,0" }
    $shortcut.Save()
}

function Test-StudyGuardianShortcut {
    param([Parameter(Mandatory = $true)][string]$ShortcutPath, [Parameter(Mandatory = $true)][string]$RootDir, [Parameter(Mandatory = $true)][string]$LauncherSwitch)
    if (-not (Test-Path -LiteralPath $ShortcutPath)) { return $false }
    try {
        $shell = New-Object -ComObject WScript.Shell
        $shortcut = $shell.CreateShortcut($ShortcutPath)
        $launcher = Join-Path $RootDir "scripts\launch-studyguardian.ps1"
        return $shortcut.TargetPath -and $shortcut.Arguments.IndexOf($launcher, [StringComparison]::OrdinalIgnoreCase) -ge 0 -and
            $shortcut.Arguments.IndexOf($LauncherSwitch, [StringComparison]::OrdinalIgnoreCase) -ge 0
    } catch { return $false }
}
