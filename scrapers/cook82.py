import asyncio
import re

import httpx

from scrapers.base import BaseScraper, Post


class Cook82Scraper(BaseScraper):
    community = "cook82"
    community_name = "82cook"
    base_url = "https://www.82cook.com"
    data_required = True

    async def _parse_views(self, client: httpx.AsyncClient, url: str) -> int:
        """개별 게시글에서 조회수 파싱"""
        try:
            soup = await self._fetch_html(client, url)
            rl = soup.find("div", class_="readLeft")
            if rl:
                m = re.search(r"조회수\s*:\s*([\d,]+)", rl.get_text())
                if m:
                    return int(m.group(1).replace(",", ""))
        except Exception:
            pass
        return 0

    async def _fetch_one(self, sem: asyncio.Semaphore, client: httpx.AsyncClient,
                         title: str, url: str) -> Post:
        """세마포어 제한 하에 개별 게시글 크롤링"""
        async with sem:
            views = await self._parse_views(client, url)
            await asyncio.sleep(0.3)
            return self._make_post(title, url, views=views)

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/entiz/enti.php?bn=15")
        entries = []

        for a_tag in soup.select("ul.most li a"):
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            entries.append((title, url))

        # 개별 게시글 병렬 크롤링 (동시 5개)
        sem = asyncio.Semaphore(5)
        tasks = [self._fetch_one(sem, client, t, u) for t, u in entries]
        posts = await asyncio.gather(*tasks)

        return list(posts)
