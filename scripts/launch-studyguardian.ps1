param(
    [string]$RootDir,
    [switch]$Background,
    [switch]$OpenControlCenter,
    [switch]$OpenQuickPanel
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
if ($Background) {
    if (-not (Test-StudyGuardianLoginStartAllowed -RootDir $RootDir)) {
        Write-StudyGuardianLog -RootDir $RootDir -Source "launcher" -Message "login start disabled by user"
        return
    }
    & (Join-Path $RootDir "scripts\start-all.ps1") -RootDir $RootDir
    return
}

$route = if ($OpenQuickPanel) { "quick-panel" } else { "control-center" }
$startArgs = @{ RootDir = $RootDir; UserLaunch = $true }
& (Join-Path $RootDir "scripts\start-all.ps1") @startArgs
$runtime = Get-StudyGuardianPetRuntime -RootDir $RootDir
$tauriExe = Join-Path $RootDir "pet-v3\StudyGuardian.exe"
if (-not (Test-Path -LiteralPath $tauriExe)) { throw "Control Center executable not found: $tauriExe" }
$arguments = @("--user-launch", "--show", $route)
if ($runtime -eq "pyqt") { $arguments = @("--no-pet") + $arguments }
Start-Process -FilePath $tauriExe -ArgumentList $arguments -WorkingDirectory (Split-Path -Parent $tauriExe)
Write-StudyGuardianLog -RootDir $RootDir -Source "launcher" -Message "requested $route; pet=$runtime"
