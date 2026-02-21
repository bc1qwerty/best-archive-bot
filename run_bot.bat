@echo off
chcp 65001 >nul
cd /d "C:\Projects\best_archive_bot"

:: NSSM 서비스로 이미 실행 중이면 중복 방지
sc query BestArchiveBot | findstr "RUNNING" >nul 2>&1
if %errorlevel%==0 (
    echo [경고] BestArchiveBot 서비스가 이미 실행 중입니다. 중복 실행을 차단합니다.
    echo        서비스 관리: nssm restart BestArchiveBot
    pause
    exit /b 1
)

:loop
echo [%date% %time%] 봇 시작...
.venv\Scripts\python main.py
echo [%date% %time%] 봇 종료됨 (종료코드: %errorlevel%), 10초 후 재시작...
timeout /t 10 /nobreak >nul
goto loop
