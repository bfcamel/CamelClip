@echo off
setlocal
rem Run the user-scoped Linux build in the default WSL distribution.
wsl.exe --cd ~ --exec sh -c "exec ~/.local/bin/camelclip-gui --browser"
if errorlevel 1 (
  echo CamelClip could not start. Check the default WSL distribution and the Linux build in ~/.local/bin.
  pause
  exit /b 1
)
