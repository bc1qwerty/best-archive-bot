import httpx

from scrapers.base import BaseScraper, Post


class BobaedreamScraper(BaseScraper):
    community = "bobaedream"
    community_name = "보배드림"
    base_url = "https://www.bobaedream.co.kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/list?code=best")
        posts = []

        for row in soup.select("tr.best"):
            a_tag = row.select_one("td.pl14 a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        # 대체 셀렉터 (사이트 구조 변경 대응)
        if not posts:
            for a_tag in soup.select("a.bsubject"):
                title = a_tag.get_text(strip=True)
                href = a_tag.get("href", "")
                if not href or not title:
                    continue
                url = href if href.startswith("http") else f"{self.base_url}{href}"
                posts.append(self._make_post(title, url))

        return posts
