@echo off
setlocal

net session >nul 2>&1
if %errorlevel% neq 0 (
  powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -Verb RunAs -FilePath '%~f0'"
  exit /b
)

powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$svc=Get-Service -Name 'gov-pass' -ErrorAction Stop; Start-Service -Name 'gov-pass' -ErrorAction Stop; $svc.WaitForStatus('Running',[TimeSpan]::FromSeconds(30))"

endlocal
