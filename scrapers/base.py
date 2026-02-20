import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass

import httpx
from bs4 import BeautifulSoup

from config.settings import MAX_POSTS_PER_COMMUNITY
from utils.http_client import fetch_html

logger = logging.getLogger(__name__)


@dataclass
class Post:
    title: str
    url: str
    community: str       # 영문 key (예: "dcinside")
    community_name: str  # 한글 이름 (예: "DC인사이드")


class BaseScraper(ABC):
    community: str = ""
    community_name: str = ""
    base_url: str = ""
    encoding: str = "utf-8"

    async def _fetch_html(self, client: httpx.AsyncClient, url: str) -> BeautifulSoup:
        """HTML 가져와서 BeautifulSoup 객체로 반환"""
        html = await fetch_html(client, url, encoding=self.encoding, referer=self.base_url)
        return BeautifulSoup(html, "html.parser")

    @abstractmethod
    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        """베스트 게시글 목록 반환 (서브클래스에서 구현)"""
        ...

    def _make_post(self, title: str, url: str) -> Post:
        """Post 객체 생성 헬퍼"""
        return Post(
            title=title.strip(),
            url=url,
            community=self.community,
            community_name=self.community_name,
        )

    async def safe_fetch(self, client: httpx.AsyncClient) -> list[Post]:
        """예외 격리 래퍼 — 실패 시 빈 리스트 반환"""
        try:
            posts = await self.fetch_best_posts(client)
            # javascript:, # 등 잘못된 URL 필터링
            posts = [p for p in posts if p.url.startswith("http")]
            posts = posts[:MAX_POSTS_PER_COMMUNITY]
            logger.info("[%s] %d개 게시글 수집", self.community_name, len(posts))
            return posts
        except Exception:
            logger.exception("[%s] 스크래핑 실패", self.community_name)
            return []
