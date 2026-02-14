@echo off
setlocal

set "DIR=%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "Start-Process -Verb RunAs -FilePath '%DIR%splitter.exe' -WorkingDirectory '%DIR%'"

endlocal

