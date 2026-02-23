import logging

import httpx
from bs4 import BeautifulSoup

from scrapers.base import BaseScraper, Post
from utils.playwright_session import extract_session

logger = logging.getLogger(__name__)


class FmkoreaScraper(BaseScraper):
    community = "fmkorea"
    community_name = "에펨코리아"
    base_url = "https://www.fmkorea.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        # 1) Playwright로 JS 챌린지 통과 → 쿠키/UA 추출
        session = await extract_session(self.base_url)
        if session is None:
            logger.warning("[%s] 세션 추출 실패, 스킵", self.community_name)
            return []

        # 2) 추출된 세션을 httpx 요청에 주입
        headers = {
            "User-Agent": session["user_agent"],
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            "Accept-Language": "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7",
            "Referer": self.base_url,
        }
        resp = await client.get(
            f"{self.base_url}/best",
            headers=headers,
            cookies=session["cookies"],
            timeout=15.0,
        )
        resp.raise_for_status()
        html = resp.text

        # 3) 응답 검증 — JS 챌린지 페이지가 아닌지 확인
        if not self._is_valid_html(html):
            logger.warning("[%s] 응답이 JS 챌린지 페이지로 판단됨", self.community_name)
            return []

        soup = BeautifulSoup(html, "lxml")
        return self._parse_posts(soup)

    @staticmethod
    def _is_valid_html(html: str) -> bool:
        """정상 HTML인지 검증 (JS 챌린지 페이지 배제)."""
        if len(html) < 2000:
            return False
        lower = html.lower()
        # 챌린지 마커 감지
        if "checking your browser" in lower or "enable javascript" in lower:
            return False
        # FmKorea 콘텐츠 존재 여부
        return "fmkorea" in lower or "li_best" in lower or "bd_lst" in lower

    def _parse_posts(self, soup: BeautifulSoup) -> list[Post]:
        posts = []

        # 베스트 카드 뷰
        for row in soup.select("li.li_best2_pop0, li.li_best2_pop1, li.li_best2_pop2"):
            a_tag = row.select_one("h3.title a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        # 대체 셀렉터 (리스트 뷰)
        if not posts:
            for row in soup.select("table.bd_lst tbody tr"):
                a_tag = row.select_one("td.title a")
                if not a_tag:
                    continue
                title = a_tag.get_text(strip=True)
                href = a_tag.get("href", "")
                if not href or not title:
                    continue
                url = href if href.startswith("http") else f"{self.base_url}{href}"
                posts.append(self._make_post(title, url))

        return posts
