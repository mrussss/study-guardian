param(
    [Parameter(ParameterSetName = "Create")][switch]$Create,
    [Parameter(ParameterSetName = "Remove")][switch]$Remove,
    [Parameter(ParameterSetName = "Get")][switch]$GetState,
    [string]$RootDir,
    [string]$DesktopDirectory
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "shortcut-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($DesktopDirectory)) { $DesktopDirectory = [Environment]::GetFolderPath("Desktop") }
$shortcutPath = Join-Path $DesktopDirectory "StudyGuardian.lnk"

if ($GetState) {
    if (Test-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-OpenControlCenter") { "enabled" } else { "disabled" }
    return
}
if ($Create) {
    Set-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-OpenControlCenter" -Description "Open StudyGuardian Control Center"
    "created"
    return
}
if ($Remove) {
    if (Test-Path -LiteralPath $shortcutPath) { Remove-Item -LiteralPath $shortcutPath -Force }
    "removed"
    return
}
throw "Specify -Create, -Remove, or -GetState"
