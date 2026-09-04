# StudyGuardian 一键停止脚本 (Windows PowerShell)

Write-Host "Stopping StudyGuardian processes..." -ForegroundColor Yellow

# Stop the lightweight crash-recovery loop first so it cannot restart a child
# while the rest of the runtime is being shut down.
Get-CimInstance Win32_Process | Where-Object {
    $_.ProcessId -ne $PID -and
    ($_.Name -eq "powershell.exe" -or $_.Name -eq "pwsh.exe") -and
    $_.CommandLine -like "*watchdog.ps1*"
} | ForEach-Object {
    Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
}

# Stop Supervisor
Get-Process -Name "study-supervisor" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# Stop Pet / Sensor Python scripts matching their full runtime paths.
# Get-CimInstance works on current Windows PowerShell 5.1 and PowerShell 7.
Get-CimInstance Win32_Process | Where-Object {
    $_.ProcessId -ne $PID -and $_.Name -eq "python.exe" -and
    ($_.CommandLine -like "*screen\server.py*" -or $_.CommandLine -like "*pet\src\main.py*")
} | ForEach-Object {
    Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
}

Write-Host "StudyGuardian stopped." -ForegroundColor Green
