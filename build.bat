@echo off
cd /d "%~dp0"
where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go was not found in PATH. Install it from https://go.dev/dl/
    pause
    exit /b 1
)
go build -ldflags="-s -w -H=windowsgui" -o EngTreeViewer.exe .\cmd\engtree
if exist "EngTreeViewer.exe" (
    echo Built: %CD%\EngTreeViewer.exe
) else (
    echo [ERROR] Build failed.
)
pause
