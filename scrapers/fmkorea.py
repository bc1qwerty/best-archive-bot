import httpx

from scrapers.base import BaseScraper, Post


class FmkoreaScraper(BaseScraper):
    community = "fmkorea"
    community_name = "에펨코리아"
    base_url = "https://www.fmkorea.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/best")
        posts = []

        for row in soup.select("li.li_best2_pop0, li.li_best2_pop1, li.li_best2_pop2"):
            a_tag = row.select_one("h3.title a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue
            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url))

        # 대체 셀렉터 (리스트 뷰)
        if not posts:
            for row in soup.select("table.bd_lst tbody tr"):
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
