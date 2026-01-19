@echo off
:: CHROTE Auto-Start Script
:: Waits for Docker, starts services, logs result

set LOGFILE=%TEMP%\chrote-startup.log
echo [%date% %time%] Starting CHROTE... >> %LOGFILE%

:: Wait for Docker Desktop to be ready (it starts on login via its own setting)
:waitdocker
docker info >nul 2>&1
if errorlevel 1 (
    echo [%date% %time%] Waiting for Docker... >> %LOGFILE%
    timeout /t 5 /nobreak >nul
    goto waitdocker
)

:: Start the services
cd /d "e:\Docker\AgentArena"
docker compose up -d chrote >> %LOGFILE% 2>&1

echo [%date% %time%] CHROTE started >> %LOGFILE%
