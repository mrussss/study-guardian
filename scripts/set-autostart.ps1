param(
    [Parameter(ParameterSetName = "Enable")][switch]$Enable,
    [Parameter(ParameterSetName = "Disable")][switch]$Disable,
    [Parameter(ParameterSetName = "Get")][switch]$GetState,
    [string]$RootDir,
    [string]$StartupDirectory
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "shortcut-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($StartupDirectory)) { $StartupDirectory = [Environment]::GetFolderPath("Startup") }
$shortcutPath = Join-Path $StartupDirectory "StudyGuardian.lnk"

if ($GetState) {
    if (Test-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-Background") { "enabled" } else { "disabled" }
    return
}
if ($Enable) {
    Set-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-Background" -Description "Start StudyGuardian after Windows logon"
    "enabled"
    return
}
if ($Disable) {
    if (Test-Path -LiteralPath $shortcutPath) { Remove-Item -LiteralPath $shortcutPath -Force }
    "disabled"
    return
}
throw "Specify -Enable, -Disable, or -GetState"
