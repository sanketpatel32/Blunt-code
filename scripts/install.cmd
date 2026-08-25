@echo off
rem One-command installer for Blunt Code from cmd.exe.
rem Fetches and runs the PowerShell installer attached to the latest GitHub release;
rem the installer verifies the SHA-256 checksum before installing anything.
setlocal
powershell -NoProfile -ExecutionPolicy Bypass -Command "Invoke-RestMethod -Uri 'https://github.com/sanketpatel32/Blunt-code/releases/latest/download/install-latest.ps1' | Invoke-Expression"
if errorlevel 1 exit /b 1
