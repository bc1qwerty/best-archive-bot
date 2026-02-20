import httpx

from scrapers.base import BaseScraper, Post


class PpomppuScraper(BaseScraper):
    community = "ppomppu"
    community_name = "뽐뿌"
    base_url = "https://www.ppomppu.co.kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/hot.php")
        posts = []

        for a_tag in soup.select(".baseList-title a"):
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            if href.startswith("http"):
                url = href
            elif href.startswith("/"):
                url = f"{self.base_url}{href}"
            else:
                url = f"{self.base_url}/{href}"
            posts.append(self._make_post(title, url))

        # 중복 URL 제거 (중첩 a 태그로 인한 중복)
        seen = set()
        unique = []
        for p in posts:
            if p.url not in seen:
                seen.add(p.url)
                unique.append(p)

        return unique
