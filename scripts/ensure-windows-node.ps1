[CmdletBinding()]
param(
    [string]$BuildRoot = "D:\StudyGuardianBuild"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
$pinnedNodeVersion = "22.22.1"
$archiveName = "node-v$pinnedNodeVersion-win-x64.zip"
$folderName = "node-v$pinnedNodeVersion-win-x64"
$BuildRoot = [IO.Path]::GetFullPath($BuildRoot)
$buildDrive = [IO.Path]::GetPathRoot($BuildRoot)
if ($buildDrive -ne "D:\") { throw "Windows tool cache must remain on D:; received $BuildRoot" }

$toolsRoot = [IO.Path]::GetFullPath((Join-Path $BuildRoot "tools"))
$nodeHome = [IO.Path]::GetFullPath((Join-Path $toolsRoot $folderName))
if (-not $nodeHome.StartsWith("$BuildRoot\", [StringComparison]::OrdinalIgnoreCase)) {
    throw "Node tool path escaped the build root: $nodeHome"
}
$nodeExe = Join-Path $nodeHome "node.exe"
$npmCmd = Join-Path $nodeHome "npm.cmd"

function Get-Sha256 {
    param([Parameter(Mandatory)] [string]$Path)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    $stream = [IO.File]::OpenRead($Path)
    try { return ([BitConverter]::ToString($sha256.ComputeHash($stream))).Replace("-", "").ToLowerInvariant() }
    finally { $stream.Dispose(); $sha256.Dispose() }
}

function Assert-PinnedNode {
    if (-not (Test-Path -LiteralPath $nodeExe -PathType Leaf) -or -not (Test-Path -LiteralPath $npmCmd -PathType Leaf)) { return $false }
    $actual = (& $nodeExe --version).Trim()
    if ($actual -ne "v$pinnedNodeVersion") { throw "Cached Node version mismatch: expected v$pinnedNodeVersion, received $actual" }
    return $true
}

if (-not (Assert-PinnedNode)) {
    New-Item -ItemType Directory -Path $toolsRoot -Force | Out-Null
    $downloadRoot = [IO.Path]::GetFullPath((Join-Path $toolsRoot "downloads"))
    $stageRoot = [IO.Path]::GetFullPath((Join-Path $toolsRoot (".node-stage-" + [guid]::NewGuid().ToString("N"))))
    foreach ($path in @($downloadRoot, $stageRoot)) {
        if (-not $path.StartsWith("$toolsRoot\", [StringComparison]::OrdinalIgnoreCase)) { throw "Unsafe tool staging path: $path" }
    }
    New-Item -ItemType Directory -Path $downloadRoot, $stageRoot -Force | Out-Null
    $archivePath = Join-Path $downloadRoot $archiveName
    $sumsPath = Join-Path $downloadRoot "SHASUMS256-$pinnedNodeVersion.txt"
    $baseUrl = "https://nodejs.org/dist/v$pinnedNodeVersion"
    try {
        Write-Host "==> Downloading pinned Node v$pinnedNodeVersion into $toolsRoot"
        Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/SHASUMS256.txt" -OutFile $sumsPath
        Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$archiveName" -OutFile $archivePath
        $pattern = "(?mi)^([a-f0-9]{64})\s+\*?" + [regex]::Escape($archiveName) + "\s*$"
        $match = [regex]::Match([IO.File]::ReadAllText($sumsPath), $pattern)
        if (-not $match.Success) { throw "Official Node checksum entry was not found for $archiveName" }
        $expectedHash = $match.Groups[1].Value.ToLowerInvariant()
        $actualHash = Get-Sha256 -Path $archivePath
        if ($actualHash -ne $expectedHash) { throw "Node archive SHA256 mismatch" }
        Expand-Archive -LiteralPath $archivePath -DestinationPath $stageRoot -Force
        $extracted = Join-Path $stageRoot $folderName
        if (-not (Test-Path -LiteralPath (Join-Path $extracted "node.exe") -PathType Leaf)) { throw "Node archive did not contain node.exe" }
        if (Test-Path -LiteralPath $nodeHome) { [IO.Directory]::Delete($nodeHome, $true) }
        Move-Item -LiteralPath $extracted -Destination $nodeHome
    } finally {
        if (Test-Path -LiteralPath $stageRoot) { [IO.Directory]::Delete($stageRoot, $true) }
    }
}
if (-not (Assert-PinnedNode)) { throw "Pinned Node installation failed" }
Write-Host "==> Windows Node pinned at v${pinnedNodeVersion}: $nodeHome"
Write-Output $nodeHome
