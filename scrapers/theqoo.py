import httpx

from scrapers.base import BaseScraper, Post


class TheqooScraper(BaseScraper):
    community = "theqoo"
    community_name = "더쿠"
    base_url = "https://theqoo.net"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/hot")
        posts = []

        for row in soup.select("table.theqoo_board_table tbody tr"):
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
