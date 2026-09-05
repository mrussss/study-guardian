param(
    [string]$RootDir,
    [switch]$Background,
    [switch]$OpenControlCenter,
    [switch]$OpenQuickPanel
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "runtime-common.ps1")
$RootDir = Resolve-StudyGuardianRoot -RootDir $RootDir -CallingScriptRoot $PSScriptRoot
& (Join-Path $RootDir "scripts\start-all.ps1") -RootDir $RootDir
if ($Background) { return }

$route = if ($OpenQuickPanel) { "quick-panel" } else { "control-center" }
$runtime = Get-StudyGuardianPetRuntime -RootDir $RootDir
$tauriExe = Join-Path $RootDir "pet-v3\StudyGuardian.exe"
if (-not (Test-Path -LiteralPath $tauriExe)) { throw "Control Center executable not found: $tauriExe" }
$arguments = @("--show", $route)
if ($runtime -eq "pyqt") { $arguments = @("--no-pet") + $arguments }
Start-Process -FilePath $tauriExe -ArgumentList $arguments -WorkingDirectory (Split-Path -Parent $tauriExe)
Write-StudyGuardianLog -RootDir $RootDir -Source "launcher" -Message "requested $route; pet=$runtime"
