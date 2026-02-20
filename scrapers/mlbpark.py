import asyncio
import logging
import re

import httpx

from scrapers.base import BaseScraper, Post

logger = logging.getLogger(__name__)

_ID_RE = re.compile(r"id=(\d+)")


class MlbparkScraper(BaseScraper):
    community = "mlbpark"
    community_name = "MLB파크"
    base_url = "https://mlbpark.donga.com"
    data_required = True

    async def _parse_post_detail(self, client: httpx.AsyncClient, url: str) -> tuple[int, int]:
        """개별 게시글에서 추천수/조회수 파싱"""
        try:
            soup = await self._fetch_html(client, url)

            votes = 0
            reco_span = soup.find("span", class_="count_recommend")
            if reco_span:
                try:
                    votes = int(reco_span.get_text(strip=True).replace(",", ""))
                except ValueError:
                    pass

            views = 0
            text2 = soup.find("div", class_="text2")
            if text2:
                m = re.search(r"조회\s*([\d,]+)", text2.get_text())
                if m:
                    views = int(m.group(1).replace(",", ""))

            return votes, views
        except Exception:
            return 0, 0

    async def _fetch_one(self, sem: asyncio.Semaphore, client: httpx.AsyncClient,
                         title: str, url: str) -> Post:
        """세마포어 제한 하에 개별 게시글 크롤링"""
        async with sem:
            votes, views = await self._parse_post_detail(client, url)
            await asyncio.sleep(0.3)
            return self._make_post(title, url, votes=votes, views=views)

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        # TODAY BEST 추천순
        soup = await self._fetch_html(client, f"{self.base_url}/mp/best.php?b=bullpen&m=like")
        entries = []
        seen = set()

        table = soup.find("table", class_="tbl_type01")
        if not table:
            return []

        tbody = table.find("tbody")
        if not tbody:
            return []

        for tr in tbody.find_all("tr"):
            tds = tr.find_all("td")
            if len(tds) < 4:
                continue
            a_tag = tds[1].find("a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not title or not href:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            if url not in seen:
                seen.add(url)
                entries.append((title, url))

        # 개별 게시글 병렬 크롤링 (동시 5개)
        sem = asyncio.Semaphore(5)
        tasks = [self._fetch_one(sem, client, t, u) for t, u in entries]
        posts = await asyncio.gather(*tasks)

        return list(posts)
