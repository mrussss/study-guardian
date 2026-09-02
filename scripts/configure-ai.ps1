# Configure the text AI endpoint. API keys are written to a user-only secret file.
param(
    [string]$ConfigPath = "D:\StudyGuardianDev\config\config.yaml",
    [string]$HelperPath = "D:\StudyGuardianDev\bin\config-helper.exe",
    [string]$Provider = "",
    [string]$Model = "",
    [string]$BaseUrl = "",
    [string]$WorkspaceId = "",
    [string]$ApiKeyEnv = "",
    [string]$ApiKeyFile = "",
    [switch]$Disable
)
$ErrorActionPreference = "Stop"

if (-not (Test-Path -LiteralPath $ConfigPath)) { throw "Config not found: $ConfigPath" }
if (-not (Test-Path -LiteralPath $HelperPath)) { throw "config-helper.exe not found: $HelperPath" }
if (-not $Provider) { $Provider = Read-Host "Provider (none/openai/deepseek/qwen/kimi/zhipu/siliconflow/doubao/ollama)" }
if ($Disable) { $Provider = "none" }
if (-not $Provider) { throw "Provider is required" }
if (-not $Model -and $Provider -ne "none") { $Model = Read-Host "Model" }
if ($Provider -eq "qwen" -and $WorkspaceId -and -not $BaseUrl) {
    $BaseUrl = "https://$WorkspaceId.cn-beijing.maas.aliyuncs.com/compatible-mode/v1"
}
if (-not $BaseUrl -and $Provider -eq "openai-compatible") { $BaseUrl = Read-Host "Base URL" }

$backup = "$ConfigPath.bak-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
Copy-Item -LiteralPath $ConfigPath -Destination $backup
$arguments = @("-config", $ConfigPath, "-provider", $Provider)
if ($Model) { $arguments += @("-model", $Model) }
if ($BaseUrl) { $arguments += @("-base-url", $BaseUrl) }
if ($ApiKeyEnv) { $arguments += @("-api-key-env", $ApiKeyEnv) }
if ($ApiKeyFile) { $arguments += @("-api-key-file", $ApiKeyFile) }

if ($Provider -ne "none" -and $Provider -ne "ollama" -and -not $ApiKeyEnv -and -not $ApiKeyFile) {
    $secretDir = Join-Path (Split-Path -Parent $ConfigPath) "secrets"
    New-Item -ItemType Directory -Path $secretDir -Force | Out-Null
    $secretPath = Join-Path $secretDir "$Provider.key"
    $secure = Read-Host "API key (hidden; leave blank to use provider environment variable)" -AsSecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        $plain = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
        if ($plain) {
            [IO.File]::WriteAllText($secretPath, $plain + [Environment]::NewLine, [Text.UTF8Encoding]::new($false))
            try {
                & icacls.exe $secretPath /inheritance:r /grant:r "$($env:USERNAME):(R)" | Out-Null
                if ($LASTEXITCODE -ne 0) { throw "icacls returned $LASTEXITCODE" }
            } catch {
                Write-Warning "Could not restrict secret ACL; inspect permissions manually: $secretPath"
            }
            $arguments += @("-api-key-file", $secretPath)
        }
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
}

& $HelperPath @arguments
if ($LASTEXITCODE -ne 0) { throw "AI configuration failed; backup preserved at $backup" }
Write-Host "AI configuration saved. Backup: $backup" -ForegroundColor Green
