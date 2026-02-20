import hashlib
import logging
from datetime import datetime, timedelta, timezone
from urllib.parse import urldefrag

import aiosqlite

from config.settings import DB_PATH, RECORD_EXPIRE_HOURS

logger = logging.getLogger(__name__)

KST = timezone(timedelta(hours=9))


class PostDatabase:
    def __init__(self):
        self.db_path = DB_PATH
        self.db_path.parent.mkdir(parents=True, exist_ok=True)

    async def init(self):
        """테이블 생성"""
        async with aiosqlite.connect(self.db_path) as db:
            await db.execute("""
                CREATE TABLE IF NOT EXISTS sent_posts (
                    url_hash TEXT PRIMARY KEY,
                    url TEXT NOT NULL,
                    community TEXT NOT NULL,
                    sent_at TEXT NOT NULL
                )
            """)
            await db.execute(
                "CREATE INDEX IF NOT EXISTS idx_sent_at ON sent_posts(sent_at)"
            )
            await db.commit()
        logger.info("DB 초기화 완료")

    @staticmethod
    def _hash_url(url: str) -> str:
        clean_url, _ = urldefrag(url)  # fragment(#...) 제거
        return hashlib.sha256(clean_url.encode()).hexdigest()

    async def filter_unsent(self, posts: list) -> list:
        """이미 전송된 게시글 필터링, 미전송 게시글만 반환"""
        if not posts:
            return []

        async with aiosqlite.connect(self.db_path) as db:
            # batch 조회: 한 번에 모든 해시 확인
            hashes = {self._hash_url(p.url): p for p in posts}
            placeholders = ",".join("?" for _ in hashes)
            cursor = await db.execute(
                f"SELECT url_hash FROM sent_posts WHERE url_hash IN ({placeholders})",
                list(hashes.keys()),
            )
            existing = {row[0] for row in await cursor.fetchall()}
            return [p for h, p in hashes.items() if h not in existing]

    async def mark_sent(self, posts: list):
        """게시글을 전송 완료로 기록"""
        if not posts:
            return

        now = datetime.now(KST).isoformat()
        async with aiosqlite.connect(self.db_path) as db:
            await db.executemany(
                "INSERT OR IGNORE INTO sent_posts (url_hash, url, community, sent_at) VALUES (?, ?, ?, ?)",
                [(self._hash_url(p.url), p.url, p.community, now) for p in posts],
            )
            await db.commit()

    async def cleanup_old_records(self):
        """만료된 레코드 삭제"""
        cutoff = (datetime.now(KST) - timedelta(hours=RECORD_EXPIRE_HOURS)).isoformat()
        async with aiosqlite.connect(self.db_path) as db:
            cursor = await db.execute(
                "DELETE FROM sent_posts WHERE sent_at < ?", (cutoff,)
            )
            await db.commit()
            if cursor.rowcount:
                logger.info("만료 레코드 %d건 삭제", cursor.rowcount)
