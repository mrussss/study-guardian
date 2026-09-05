param([string]$RootDir)
& (Join-Path $PSScriptRoot "set-autostart.ps1") -Disable -RootDir $RootDir
