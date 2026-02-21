import asyncio
import atexit
import logging
import os
import re
import sys
from logging.handlers import RotatingFileHandler
from pathlib import Path

from config.settings import BASE_DIR, LOG_DIR, BOT_TOKEN, CHAT_ID
from db.database import post_db

PID_FILE = BASE_DIR / "data" / "bot.pid"


class TokenFilter(logging.Filter):
    """로그에서 봇 토큰을 마스킹"""
    _pattern = re.compile(r"\d+:[A-Za-z0-9_-]{30,}")

    def filter(self, record):
        if isinstance(record.msg, str):
            record.msg = self._pattern.sub("[TOKEN_REDACTED]", record.msg)
        if record.args:
            args = []
            for a in record.args:
                if isinstance(a, str):
                    a = self._pattern.sub("[TOKEN_REDACTED]", a)
                args.append(a)
            record.args = tuple(args)
        return True


def setup_logging():
    """로깅 설정"""
    fmt = "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
    handlers = [
        logging.StreamHandler(sys.stdout),
        RotatingFileHandler(
            LOG_DIR / "bot.log", encoding="utf-8",
            maxBytes=10 * 1024 * 1024,  # 10MB
            backupCount=3,
        ),
    ]
    logging.basicConfig(level=logging.INFO, format=fmt, handlers=handlers)

    # 토큰 마스킹 필터 적용
    token_filter = TokenFilter()
    for handler in logging.root.handlers:
        handler.addFilter(token_filter)

    # httpx 노이즈 억제
    logging.getLogger("httpx").setLevel(logging.WARNING)


async def init_db():
    await post_db.init()


def _check_single_instance(logger):
    """Named Mutex로 다중 실행 방지 (Windows 전용)"""
    if sys.platform != "win32":
        logger.info("비-Windows 환경, Named Mutex 스킵")
        return

    import ctypes
    global _mutex_handle  # GC 방지 — 프로세스 종료 시 OS가 자동 해제

    kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    mutex_name = "Global\\BestArchiveBot_SingleInstance"
    _mutex_handle = kernel32.CreateMutexW(None, False, mutex_name)
    if ctypes.get_last_error() == 183:  # ERROR_ALREADY_EXISTS
        kernel32.CloseHandle(_mutex_handle)
        _mutex_handle = None
        logger.error("봇이 이미 실행 중입니다. 중복 실행 차단.")
        sys.exit(1)

    # PID 파일도 유지 (모니터링용)
    PID_FILE.parent.mkdir(parents=True, exist_ok=True)
    PID_FILE.write_text(str(os.getpid()))
    atexit.register(lambda: PID_FILE.unlink(missing_ok=True))


def main():
    setup_logging()
    logger = logging.getLogger(__name__)

    if not BOT_TOKEN or BOT_TOKEN == "your_bot_token_here":
        logger.error(".env에 BOT_TOKEN을 설정해주세요")
        sys.exit(1)
    if not CHAT_ID or CHAT_ID == "your_chat_id_here":
        logger.error(".env에 CHAT_ID를 설정해주세요")
        sys.exit(1)

    # 다중 실행 방지
    _check_single_instance(logger)

    # Python 3.14+ 이벤트 루프 호환
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)

    # DB 초기화
    loop.run_until_complete(init_db())

    # 봇 실행
    from bot.telegram_bot import create_application
    app = create_application()

    logger.info("봇 시작! (PID %d)", os.getpid())
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
