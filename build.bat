@echo off
cd /d "%~dp0"
where go >nul 2>&1
if errorlevel 1 (
    echo [ERROR] Go was not found in PATH. Install it from https://go.dev/dl/
    pause
    exit /b 1
)
go build -ldflags="-s -w -H=windowsgui" -o EngTreeViewer.exe .\cmd\engtree
go build -ldflags="-s -w -H=windowsgui" -o EngTreeCorte.exe .\cmd\engtreecorte
if exist "EngTreeViewer.exe" (
    echo Built: %CD%\EngTreeViewer.exe
) else (
    echo [ERROR] EngTreeViewer.exe failed.
)
if exist "EngTreeCorte.exe" (
    echo Built: %CD%\EngTreeCorte.exe
) else (
    echo [ERROR] EngTreeCorte.exe failed.
)
pause
