# StudyGuardian 一键停止脚本 (Windows PowerShell)

Write-Host "Stopping StudyGuardian processes..." -ForegroundColor Yellow

# Stop Supervisor
Get-Process -Name "study-supervisor" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# Stop Pet / Sensor Python scripts matching keywords
Get-WmiObject Win32_Process | Where-Object {
    $_.CommandLine -like "*screen\server.py*" -or $_.CommandLine -like "*pet\src\main.py*"
} | ForEach-Object {
    Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue
}

Write-Host "StudyGuardian stopped." -ForegroundColor Green
