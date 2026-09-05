param([string]$RootDir)
& (Join-Path $PSScriptRoot "set-autostart.ps1") -Enable -RootDir $RootDir
