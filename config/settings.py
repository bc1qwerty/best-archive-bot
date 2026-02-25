import os
from pathlib import Path

from dotenv import load_dotenv

BASE_DIR = Path(__file__).resolve().parent.parent
load_dotenv(BASE_DIR / ".env")

BOT_TOKEN: str = os.getenv("BOT_TOKEN", "")
CHAT_ID: str = os.getenv("CHAT_ID", "")
ADMIN_ID: int = int(os.getenv("ADMIN_ID", "0"))

# 스크래핑 설정
MAX_CONCURRENT_REQUESTS = 4
REQUEST_DELAY_MIN = 1.0
REQUEST_DELAY_MAX = 3.0
MAX_POSTS_PER_COMMUNITY = 20
MIN_VOTES = 100     # 추천수 필터 (0=미제공 사이트는 면제)
MIN_VIEWS = 20000   # 조회수 필터 (추천수 없는 사이트용)
MIN_COMMENTS = 150  # 댓글수 필터

# DB 설정
DB_PATH = BASE_DIR / "data" / "posts.db"
RECORD_EXPIRE_HOURS = 24 * 7  # 7일

