# Remove the per-user StudyGuardian logon shortcut.
$ErrorActionPreference = "Stop"

$ShortcutPath = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\Startup\StudyGuardian.lnk"
if (Test-Path -LiteralPath $ShortcutPath) {
    Remove-Item -LiteralPath $ShortcutPath -Force
    Write-Host "StudyGuardian startup shortcut removed." -ForegroundColor Green
} else {
    Write-Host "StudyGuardian startup shortcut was not installed." -ForegroundColor Yellow
}
