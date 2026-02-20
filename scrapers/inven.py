import httpx

from scrapers.base import BaseScraper, Post


class InvenScraper(BaseScraper):
    community = "inven"
    community_name = "인벤"
    base_url = "https://www.inven.co.kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/board/webzine/2097")
        posts = []

        for a_tag in soup.select("td.tit a.subject-link"):
            # 공지 제외
            tr = a_tag.find_parent("tr")
            if tr and "notice" in (tr.get("class") or []):
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        return posts
