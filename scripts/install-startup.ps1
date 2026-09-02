# Install a per-user Windows logon shortcut. No administrator permission is required.
$ErrorActionPreference = "Stop"

$RootDir = "D:\StudyGuardianDev"
$StartupDir = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup"
$ShortcutPath = Join-Path $StartupDir "StudyGuardian.lnk"
$StartScript = Join-Path $RootDir "scripts\start-all.ps1"

if (-not (Test-Path -LiteralPath $StartScript)) {
    throw "StudyGuardian start script not found: $StartScript"
}

New-Item -ItemType Directory -Path $StartupDir -Force | Out-Null
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut($ShortcutPath)
$shortcut.TargetPath = (Get-Command powershell.exe).Source
$shortcut.Arguments = "-NoProfile -WindowStyle Hidden -ExecutionPolicy Bypass -File `"$StartScript`""
$shortcut.WorkingDirectory = $RootDir
$shortcut.Description = "Start StudyGuardian after Windows logon"
$shortcut.Save()

Write-Host "StudyGuardian startup shortcut installed: $ShortcutPath" -ForegroundColor Green
