@echo off
cd /d "%~dp0"

echo [1/3] Xoa ban build cu...
if exist "LocalAIProxy.exe" del /f /q "LocalAIProxy.exe"
if exist "LocalAIProxy.exe" (
    echo Khong xoa duoc LocalAIProxy.exe - hay tat app dang chay roi thu lai.
    pause
    exit /b 1
)
if exist "build\bin" rmdir /s /q "build\bin"

echo [2/3] Build ban moi...
where wails >nul 2>nul
if %errorlevel%==0 (
    wails build -clean
) else (
    go run github.com/wailsapp/wails/v2/cmd/wails@v2.15.0 build -clean
)
if %errorlevel% neq 0 (
    echo.
    echo BUILD THAT BAI
    pause
    exit /b 1
)

echo [3/3] Chuyen exe ra thu muc du an...
move /y "build\bin\LocalAIProxy.exe" "LocalAIProxy.exe" >nul
rmdir /s /q "build\bin"

echo.
echo XONG: %~dp0LocalAIProxy.exe
pause
