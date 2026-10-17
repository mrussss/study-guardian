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
$preferencePath = Join-Path $RootDir "config\autostart-state.json"
function Save-AutostartPreference([bool]$Enabled) {
    $configPath = Split-Path -Parent $preferencePath
    New-Item -ItemType Directory -Path $configPath -Force | Out-Null
    $temporary = "$preferencePath.$PID.tmp"
    @{ enabled = $Enabled } | ConvertTo-Json -Compress | Set-Content -LiteralPath $temporary -Encoding UTF8
    Move-Item -LiteralPath $temporary -Destination $preferencePath -Force
}

if ($GetState) {
    if (Test-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-Background") { "enabled" } else { "disabled" }
    return
}
if ($Enable) {
    Set-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-Background" -Description "Start StudyGuardian after Windows logon"
    if (-not (Test-StudyGuardianShortcut -ShortcutPath $shortcutPath -RootDir $RootDir -LauncherSwitch "-Background")) { throw "Startup shortcut could not be verified" }
    Save-AutostartPreference $true
    "enabled"
    return
}
if ($Disable) {
    Save-AutostartPreference $false
    if (Test-Path -LiteralPath $shortcutPath) { Remove-Item -LiteralPath $shortcutPath -Force }
    if (Test-Path -LiteralPath $shortcutPath) { throw "Startup shortcut could not be removed" }
    "disabled"
    return
}
throw "Specify -Enable, -Disable, or -GetState"
