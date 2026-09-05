param([string]$RepoRoot, [string]$OutputPath)

$ErrorActionPreference = "Stop"
if ([string]::IsNullOrWhiteSpace($RepoRoot)) { $RepoRoot = Split-Path -Parent $PSScriptRoot }
$RepoRoot = [IO.Path]::GetFullPath($RepoRoot)
$sourcePetRoot = Join-Path $RepoRoot "pet-v3"
if ([string]::IsNullOrWhiteSpace($OutputPath)) { $OutputPath = Join-Path $RepoRoot "dist\windows\pet-v3\StudyGuardian.exe" }
$OutputPath = [IO.Path]::GetFullPath($OutputPath)
$npm = Get-Command npm.cmd -ErrorAction Stop
$cargo = Get-Command cargo.exe -ErrorAction SilentlyContinue
if (-not $cargo) {
    $cargoCandidate = "D:\develop\Rust\cargo\bin\cargo.exe"
    if (Test-Path -LiteralPath $cargoCandidate) { $env:PATH = "$(Split-Path -Parent $cargoCandidate);$env:PATH" }
}
if (-not (Get-Command cargo.exe -ErrorAction SilentlyContinue)) { throw "Windows Rust toolchain is unavailable" }

$documentsRoot = [IO.Path]::GetFullPath([Environment]::GetFolderPath("MyDocuments"))
$buildBase = [IO.Path]::GetFullPath((Join-Path $documentsRoot ".StudyGuardianBuild"))
$targetRoot = Join-Path $buildBase "target"
$petRoot = [IO.Path]::GetFullPath((Join-Path $buildBase "build-source"))
if (-not $petRoot.StartsWith("$buildBase\", [StringComparison]::OrdinalIgnoreCase)) { throw "Unsafe build staging path" }
if (Test-Path -LiteralPath $petRoot) { Remove-Item -LiteralPath $petRoot -Recurse -Force }
New-Item -ItemType Directory -Path $petRoot, (Join-Path $petRoot "src-tauri") -Force | Out-Null
foreach ($name in @("control-center.html", "index.html", "package-lock.json", "package.json", "quick-panel.html", "tsconfig.json", "vite.config.ts")) {
    Copy-Item -LiteralPath (Join-Path $sourcePetRoot $name) -Destination (Join-Path $petRoot $name)
}
foreach ($name in @("diagnostics", "src")) { Copy-Item -LiteralPath (Join-Path $sourcePetRoot $name) -Destination $petRoot -Recurse }
foreach ($name in @("build.rs", "Cargo.lock", "Cargo.toml", "tauri.conf.json")) {
    Copy-Item -LiteralPath (Join-Path $sourcePetRoot "src-tauri\$name") -Destination (Join-Path $petRoot "src-tauri\$name")
}
foreach ($name in @("capabilities", "gen", "icons", "src")) {
    Copy-Item -LiteralPath (Join-Path $sourcePetRoot "src-tauri\$name") -Destination (Join-Path $petRoot "src-tauri") -Recurse
}
New-Item -ItemType Directory -Path $targetRoot -Force | Out-Null
$env:CARGO_INCREMENTAL = "0"
$env:CARGO_TARGET_DIR = $targetRoot
if ($petRoot -match '[&|<>^"]') { throw "Repository path contains characters unsupported by the Windows build helper" }
# cmd.exe's pushd assigns a temporary drive to the WSL UNC path. npm's Windows
# shims cannot otherwise use a UNC current directory.
$buildCommand = "pushd `"$petRoot`" && `"$($npm.Source)`" ci && `"$($npm.Source)`" run build && `"$($npm.Source)`" run tauri -- build --no-bundle"
& cmd.exe /d /s /c $buildCommand
if ($LASTEXITCODE -ne 0) { throw "Tauri production build failed" }

$built = Join-Path $targetRoot "release\studyguardian-pet-v3.exe"
if (-not (Test-Path -LiteralPath $built)) { throw "Tauri executable was not produced: $built" }
New-Item -ItemType Directory -Path (Split-Path -Parent $OutputPath) -Force | Out-Null
Copy-Item -LiteralPath $built -Destination $OutputPath -Force
if ((Get-Item -LiteralPath $OutputPath).Length -le 0) { throw "Tauri executable artifact is empty" }
$sha256 = [Security.Cryptography.SHA256]::Create()
$stream = [IO.File]::OpenRead($OutputPath)
try { $hash = ([BitConverter]::ToString($sha256.ComputeHash($stream))).Replace("-", "").ToLowerInvariant() } finally { $stream.Dispose(); $sha256.Dispose() }
Write-Host "Tauri artifact: $OutputPath"
Write-Host "SHA256: $hash"
Remove-Item -LiteralPath $petRoot -Recurse -Force
