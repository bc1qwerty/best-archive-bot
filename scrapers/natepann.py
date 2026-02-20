import re

import httpx

from scrapers.base import BaseScraper, Post


class NatepannScraper(BaseScraper):
    community = "natepann"
    community_name = "네이트판"
    base_url = "https://pann.nate.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/talk/ranking")
        posts = []

        # dt > h2 > a[href=/talk/숫자] 패턴
        for a_tag in soup.find_all("a", href=re.compile(r"^/talk/\d+")):
            if a_tag.parent and a_tag.parent.name == "h2":
                title = a_tag.get_text(strip=True)
                href = a_tag["href"]
                if not title:
                    continue
                url = f"{self.base_url}{href}"
                posts.append(self._make_post(title, url))

        return posts
