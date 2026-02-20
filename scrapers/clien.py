import httpx

from scrapers.base import BaseScraper, Post


class ClienScraper(BaseScraper):
    community = "clien"
    community_name = "클리앙"
    base_url = "https://www.clien.net"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/service/board/park")
        posts = []

        for item in soup.select("div.list_item"):
            # 공지 스킵
            if "notice" in item.get("class", []):
                continue
            a_tag = item.select_one("a.list_subject")
            if not a_tag:
                continue
            title_span = a_tag.select_one("span.subject_fixed")
            title = title_span.get_text(strip=True) if title_span else a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        return posts
