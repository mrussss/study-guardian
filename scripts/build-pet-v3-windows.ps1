[CmdletBinding()]
param(
    [string]$RepoRoot,
    [string]$OutputPath,
    [ValidateSet("Debug", "Release")]
    [string]$Configuration = "Release",
    [string]$BuildRoot = $(if ($env:STUDYGUARDIAN_BUILD_ROOT) { $env:STUDYGUARDIAN_BUILD_ROOT } else { "D:\StudyGuardianBuild" }),
    [switch]$RunFrontendTests,
    [switch]$RunRustTests,
    [string]$GitCommit = "unknown",
    [ValidateSet("true", "false", "unknown")]
    [string]$GitDirty = "unknown"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Invoke-Checked {
    param(
        [Parameter(Mandatory)] [string]$FilePath,
        [Parameter(Mandatory)] [string[]]$Arguments,
        [Parameter(Mandatory)] [string]$WorkingDirectory,
        [Parameter(Mandatory)] [string]$Label
    )
    Write-Host "==> $Label"
    $exitCode = 0
    Push-Location $WorkingDirectory
    try {
        & $FilePath @Arguments
        $exitCode = $LASTEXITCODE
    } finally {
        Pop-Location
    }
    if ($exitCode -ne 0) { throw "$Label failed with exit code $exitCode" }
}

function Invoke-Robocopy {
    param(
        [Parameter(Mandatory)] [string]$Source,
        [Parameter(Mandatory)] [string]$Destination,
        [string[]]$ExcludedDirectories = @()
    )
    $arguments = @($Source, $Destination, "/MIR", "/R:1", "/W:1", "/XJ", "/NFL", "/NDL", "/NJH", "/NJS", "/NP")
    if ($ExcludedDirectories.Count -gt 0) { $arguments += "/XD"; $arguments += $ExcludedDirectories }
    & robocopy @arguments
    if ($LASTEXITCODE -ge 8) { throw "robocopy failed with exit code $LASTEXITCODE for $Source" }
}

function Get-Sha256 {
    param([Parameter(Mandatory)] [string]$Path)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Path)
    try {
        return ([BitConverter]::ToString($sha256.ComputeHash($stream))).Replace("-", "")
    } finally {
        $stream.Dispose()
        $sha256.Dispose()
    }
}

function Get-DependencyFingerprint {
    param([Parameter(Mandatory)] [string]$PetRoot)
    $packageHash = Get-Sha256 -Path (Join-Path $PetRoot "package.json")
    $lockHash = Get-Sha256 -Path (Join-Path $PetRoot "package-lock.json")
    return "$packageHash`:$lockHash`:node-22.22.1"
}

if ([string]::IsNullOrWhiteSpace($RepoRoot)) { $RepoRoot = Split-Path -Parent $PSScriptRoot }
$RepoRoot = [IO.Path]::GetFullPath($RepoRoot)
$BuildRoot = [IO.Path]::GetFullPath($BuildRoot)
$sourcePetRoot = Join-Path $RepoRoot "pet-v3"
$sourceAssetRoot = Join-Path $RepoRoot "pet\assets"
$stagingRepo = Join-Path $BuildRoot "staging\repo"
$petRoot = Join-Path $stagingRepo "pet-v3"
$assetRoot = Join-Path $stagingRepo "pet\assets"
$stateRoot = Join-Path $BuildRoot "state"
$npmCache = Join-Path $BuildRoot "npm-cache"
$configurationName = $Configuration.ToLowerInvariant()
$targetRoot = Join-Path $BuildRoot "cargo-target\$configurationName-cache"
$artifactRoot = Join-Path $BuildRoot "artifacts\pet-v3\$configurationName"
$artifactPath = Join-Path $artifactRoot "StudyGuardian.exe"
$manifestPath = Join-Path $artifactRoot "build-manifest.json"
$dependencyStamp = Join-Path $stateRoot "pet-v3-node-dependencies.sha256"

foreach ($path in @($stagingRepo, $petRoot, $assetRoot, $stateRoot, $npmCache, $targetRoot, $artifactRoot)) {
    $full = [IO.Path]::GetFullPath($path)
    if (-not ($full -eq $BuildRoot -or $full.StartsWith("$BuildRoot\", [StringComparison]::OrdinalIgnoreCase))) {
        throw "Build path escaped the configured build root: $full"
    }
}
if (-not (Test-Path -LiteralPath $sourcePetRoot -PathType Container)) { throw "Pet source is missing: $sourcePetRoot" }
if (-not (Test-Path -LiteralPath $sourceAssetRoot -PathType Container)) { throw "Pet assets are missing: $sourceAssetRoot" }

New-Item -ItemType Directory -Path $petRoot, $assetRoot, $stateRoot, $npmCache, $targetRoot, $artifactRoot -Force | Out-Null
Write-Host "Syncing source into persistent staging: $petRoot"
Invoke-Robocopy -Source $sourcePetRoot -Destination $petRoot -ExcludedDirectories @(".git", "node_modules", "target", "dist")
Invoke-Robocopy -Source $sourceAssetRoot -Destination $assetRoot -ExcludedDirectories @(".git", "node_modules", "target", "dist", "__pycache__")

$nodeToolScript = Join-Path $RepoRoot "scripts\ensure-windows-node.ps1"
if (-not (Test-Path -LiteralPath $nodeToolScript -PathType Leaf)) { throw "Pinned Node bootstrap is missing: $nodeToolScript" }
$nodeHome = (& $nodeToolScript -BuildRoot $BuildRoot | Select-Object -Last 1)
$nodeExe = Join-Path $nodeHome "node.exe"
$npm = Join-Path $nodeHome "npm.cmd"
$env:PATH = "$nodeHome;$env:PATH"
$nodeVersion = (& $nodeExe --version).Trim()
if ($nodeVersion -ne "v22.22.1") { throw "Windows Node must be v22.22.1; received $nodeVersion" }

$cargo = Get-Command cargo.exe -ErrorAction SilentlyContinue
if (-not $cargo) {
    $cargoCandidate = "D:\develop\Rust\cargo\bin\cargo.exe"
    if (Test-Path -LiteralPath $cargoCandidate) {
        $env:PATH = "$(Split-Path -Parent $cargoCandidate);$env:PATH"
        $cargo = Get-Command cargo.exe -ErrorAction SilentlyContinue
    }
}
if (-not $cargo) { throw "Windows Rust toolchain is unavailable" }
Push-Location $petRoot
try {
    $rustVersion = (& rustc.exe --version).Trim()
    $cargoVersion = (& $cargo.Source --version).Trim()
} finally { Pop-Location }
if (-not $rustVersion.StartsWith("rustc 1.98.1 ")) { throw "Windows Rust must be 1.98.1; received $rustVersion" }
Write-Host "==> Toolchain: Node $nodeVersion; $rustVersion; $cargoVersion"

$env:npm_config_cache = $npmCache
$env:CARGO_TARGET_DIR = $targetRoot
$env:CARGO_INCREMENTAL = $(if ($Configuration -eq "Debug") { "1" } else { "0" })

$fingerprint = Get-DependencyFingerprint -PetRoot $petRoot
$installedFingerprint = if (Test-Path -LiteralPath $dependencyStamp) { (Get-Content -LiteralPath $dependencyStamp -Raw).Trim() } else { "" }
$nodeModules = Join-Path $petRoot "node_modules"
if (-not (Test-Path -LiteralPath $nodeModules -PathType Container) -or $fingerprint -ne $installedFingerprint) {
    Invoke-Checked -FilePath $npm -Arguments @("ci") -WorkingDirectory $petRoot -Label "npm ci (dependency files changed)"
    Set-Content -LiteralPath $dependencyStamp -Value $fingerprint -NoNewline
} else {
    Write-Host "==> npm dependencies unchanged; reusing $nodeModules"
}

if ($RunFrontendTests) {
    Invoke-Checked -FilePath $npm -Arguments @("test") -WorkingDirectory $petRoot -Label "npm test"
}
if ($RunRustTests) {
    $cargoManifest = Join-Path $petRoot "src-tauri\Cargo.toml"
    Invoke-Checked -FilePath $cargo.Source -Arguments @("test", "--locked", "--manifest-path", $cargoManifest) -WorkingDirectory $petRoot -Label "cargo test --locked"
}

$tauriArguments = @("run", "tauri", "--", "build", "--no-bundle")
if ($Configuration -eq "Debug") { $tauriArguments += "--debug" }
Invoke-Checked -FilePath $npm -Arguments $tauriArguments -WorkingDirectory $petRoot -Label "Tauri $Configuration build"

$profileDirectory = if ($Configuration -eq "Debug") { "debug" } else { "release" }
$built = Join-Path $targetRoot "$profileDirectory\studyguardian-pet-v3.exe"
if (-not (Test-Path -LiteralPath $built -PathType Leaf)) { throw "Tauri executable was not produced: $built" }
Copy-Item -LiteralPath $built -Destination $artifactPath -Force
$hash = Get-Sha256 -Path $artifactPath
$buildManifest = [ordered]@{
    product = "StudyGuardian Pet v3"
    configuration = $Configuration
    git_commit = $GitCommit
    git_dirty = $GitDirty
    built_at = (Get-Date).ToString("o")
    node = $nodeVersion
    cargo = $cargoVersion
    sha256 = $hash
    artifact = $artifactPath
}
$buildManifest | ConvertTo-Json | Set-Content -LiteralPath $manifestPath -Encoding UTF8

if (-not [string]::IsNullOrWhiteSpace($OutputPath)) {
    $OutputPath = [IO.Path]::GetFullPath($OutputPath)
    New-Item -ItemType Directory -Path (Split-Path -Parent $OutputPath) -Force | Out-Null
    Copy-Item -LiteralPath $artifactPath -Destination $OutputPath -Force
    Copy-Item -LiteralPath $manifestPath -Destination (Join-Path (Split-Path -Parent $OutputPath) "build-manifest.json") -Force
}

Write-Host "Tauri artifact: $artifactPath"
Write-Host "Build manifest: $manifestPath"
Write-Host "SHA256: $hash"
