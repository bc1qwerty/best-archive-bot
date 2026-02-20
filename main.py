import asyncio
import logging
import re
import sys
from logging.handlers import RotatingFileHandler

from config.settings import LOG_DIR, BOT_TOKEN, CHAT_ID
from db.database import post_db


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


def main():
    setup_logging()
    logger = logging.getLogger(__name__)

    if not BOT_TOKEN or BOT_TOKEN == "your_bot_token_here":
        logger.error(".env에 BOT_TOKEN을 설정해주세요")
        sys.exit(1)
    if not CHAT_ID or CHAT_ID == "your_chat_id_here":
        logger.error(".env에 CHAT_ID를 설정해주세요")
        sys.exit(1)

    # Python 3.14+ 이벤트 루프 호환
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)

    # DB 초기화
    loop.run_until_complete(init_db())

    # 봇 실행
    from bot.telegram_bot import create_application
    app = create_application()

    logger.info("봇 시작!")
    app.run_polling(drop_pending_updates=True)


if __name__ == "__main__":
    main()
