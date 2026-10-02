@echo off
rem One-click build for AntennaDeck.
rem Builds in a temporary folder (Windows "Controlled folder access" blocks
rem compilers from writing inside Documents) and puts the finished program in
rem %LOCALAPPDATA%\Programs\AntennaDeck.
setlocal
cd /d "%~dp0"
set "PATH=%PATH%;%USERPROFILE%\go\bin"

where go >nul 2>nul || (
  echo Go is not installed. Download it from https://go.dev/dl/ and run this again.
  pause
  exit /b 1
)
where wails >nul 2>nul || (
  echo Installing the Wails build tool, one time only...
  go install github.com/wailsapp/wails/v2/cmd/wails@latest || goto fail
)

set "WORK=%TEMP%\AntennaDeck-build"
set "OUT=%LOCALAPPDATA%\Programs\AntennaDeck"
if exist "%WORK%" rmdir /s /q "%WORK%"
robocopy "%~dp0." "%WORK%" /E /XD "%~dp0build\bin" .git /XF AntennaDeck.exe /NFL /NDL /NJH /NJS >nul

pushd "%WORK%"
go mod tidy || goto fail
go test . || goto fail
wails build -clean -trimpath || goto fail
popd

:waitclose
tasklist /fi "imagename eq AntennaDeck.exe" | find /i "AntennaDeck.exe" >nul && (
  echo AntennaDeck is running. Close it, then press a key to install the new version.
  pause >nul
  goto waitclose
)
if not exist "%OUT%" mkdir "%OUT%"
copy /y "%WORK%\build\bin\AntennaDeck.exe" "%OUT%\" >nul || goto fail
echo.
echo Done: %OUT%\AntennaDeck.exe
explorer "%OUT%"
pause
exit /b 0

:fail
popd 2>nul
echo.
echo BUILD FAILED - see the messages above.
pause
exit /b 1
