@echo off
setlocal

set "GOV_PASS_TUI=%~dp0gov-pass-tui.exe"
powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "$ErrorActionPreference='Stop'; Start-Process -Verb RunAs -FilePath $env:GOV_PASS_TUI -WorkingDirectory (Split-Path -LiteralPath $env:GOV_PASS_TUI -Parent)"

exit /b %errorlevel%
