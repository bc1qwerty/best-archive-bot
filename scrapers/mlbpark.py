import re

import httpx

from scrapers.base import BaseScraper, Post

# 실제 게시글 링크 패턴: b=bullpen&id=숫자
_POST_PATTERN = re.compile(r"b=bullpen.*id=\d+")


class MlbparkScraper(BaseScraper):
    community = "mlbpark"
    community_name = "MLB파크"
    base_url = "https://mlbpark.donga.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        page_url = f"{self.base_url}/mp/b.php?b=bullpen&m=bbs&s=hot"
        soup = await self._fetch_html(client, page_url)
        posts = []
        seen = set()

        for a_tag in soup.find_all("a", href=_POST_PATTERN):
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not title or len(title) < 2:
                continue
            post_url = href if href.startswith("http") else f"{self.base_url}{href}"
            if post_url not in seen:
                seen.add(post_url)
                posts.append(self._make_post(title, post_url))

        return posts
