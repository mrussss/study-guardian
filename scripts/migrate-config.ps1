# Migrate a legacy flat ai block to schema v2 without touching other settings.
param(
    [string]$ConfigPath = "D:\StudyGuardianDev\config\config.yaml",
    [string]$HelperPath = "D:\StudyGuardianDev\bin\config-helper.exe"
)
$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $ConfigPath)) { throw "Config not found: $ConfigPath" }
if (-not (Test-Path -LiteralPath $HelperPath)) { throw "config-helper.exe not found: $HelperPath" }
$backup = "$ConfigPath.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
Copy-Item -LiteralPath $ConfigPath -Destination $backup
& $HelperPath -config $ConfigPath -migrate
if ($LASTEXITCODE -ne 0) { throw "Config migration failed; backup preserved at $backup" }
Write-Host "Migrated config to AI schema v2. Backup: $backup" -ForegroundColor Green
