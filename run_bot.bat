@echo off
chcp 65001 >nul
cd /d "C:\Projects\best_archive_bot"

:loop
echo [%date% %time%] 봇 시작...
.venv\Scripts\python main.py
echo [%date% %time%] 봇 종료됨 (종료코드: %errorlevel%), 10초 후 재시작...
timeout /t 10 /nobreak >nul
goto loop
