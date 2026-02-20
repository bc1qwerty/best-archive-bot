import html
import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass

import httpx
from bs4 import BeautifulSoup

from config.settings import MAX_POSTS_PER_COMMUNITY, MIN_COMMENTS, MIN_VIEWS, MIN_VOTES
from utils.http_client import fetch_html

logger = logging.getLogger(__name__)


@dataclass
class Post:
    title: str
    url: str
    community: str       # 영문 key (예: "dcinside")
    community_name: str  # 한글 이름 (예: "DC인사이드")
    votes: int = 0       # 추천수 (0 = 미제공)
    views: int = 0       # 조회수 (0 = 미제공)
    comments: int = 0    # 댓글수 (0 = 미제공)


class BaseScraper(ABC):
    community: str = ""
    community_name: str = ""
    base_url: str = ""
    encoding: str = "utf-8"
    data_required: bool = False  # True: 0/0이면 필터링 (개별 페이지 크롤링 실패 방지)

    async def _fetch_html(self, client: httpx.AsyncClient, url: str) -> BeautifulSoup:
        """HTML 가져와서 BeautifulSoup 객체로 반환"""
        html = await fetch_html(client, url, encoding=self.encoding, referer=self.base_url)
        return BeautifulSoup(html, "lxml")

    @abstractmethod
    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        """베스트 게시글 목록 반환 (서브클래스에서 구현)"""
        ...

    def _make_post(self, title: str, url: str, votes: int = 0, views: int = 0,
                   comments: int = 0) -> Post:
        """Post 객체 생성 헬퍼"""
        return Post(
            title=html.unescape(title.strip()),
            url=url,
            community=self.community,
            community_name=self.community_name,
            votes=votes,
            views=views,
            comments=comments,
        )

    def _should_include(self, p: Post) -> bool:
        """인기도 필터: 추천수/조회수/댓글수 중 하나 충족 시 통과"""
        has_votes = p.votes > 0
        has_views = p.views > 0
        has_comments = p.comments > 0
        if not has_votes and not has_views and not has_comments:
            if self.data_required:
                return False  # 개별 크롤링 실패 → 필터링
            return True  # 데이터 미제공 사이트 → 면제
        if has_votes and p.votes >= MIN_VOTES:
            return True
        if has_views and p.views >= MIN_VIEWS:
            return True
        if has_comments and p.comments >= MIN_COMMENTS:
            return True
        return False

    async def safe_fetch(self, client: httpx.AsyncClient) -> list[Post]:
        """예외 격리 래퍼 — 실패 시 빈 리스트 반환 (HTTP 상태 에러는 전파)"""
        try:
            posts = await self.fetch_best_posts(client)
            posts = [p for p in posts if p.url.startswith("http")]
            posts = [p for p in posts if self._should_include(p)]
            posts = posts[:MAX_POSTS_PER_COMMUNITY]
            logger.info("[%s] %d개 게시글 수집", self.community_name, len(posts))
            return posts
        except httpx.HTTPStatusError:
            raise
        except Exception:
            logger.exception("[%s] 스크래핑 실패", self.community_name)
            return []
