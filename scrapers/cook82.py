import httpx

from scrapers.base import BaseScraper, Post


class Cook82Scraper(BaseScraper):
    community = "cook82"
    community_name = "82cook"
    base_url = "https://www.82cook.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/entiz/enti.php?bn=15")
        posts = []

        for a_tag in soup.select("ul.most li a"):
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        return posts
