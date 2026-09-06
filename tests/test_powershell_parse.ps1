Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$files = @(Get-ChildItem -LiteralPath (Join-Path $repoRoot "scripts"), (Join-Path $repoRoot "tests") -Filter "*.ps1" -File -Recurse)
$failures = @()
foreach ($file in $files) {
    $tokens = $null
    $errors = $null
    [Management.Automation.Language.Parser]::ParseFile($file.FullName, [ref]$tokens, [ref]$errors) | Out-Null
    foreach ($error in $errors) {
        $failures += "$($file.FullName):$($error.Extent.StartLineNumber): $($error.Message)"
    }
}
if ($failures.Count -gt 0) { throw ($failures -join [Environment]::NewLine) }
Write-Host "PASS: parsed $($files.Count) PowerShell scripts"
