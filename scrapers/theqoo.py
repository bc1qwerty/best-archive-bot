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
            tds = row.select("td")
            if len(tds) < 3:
                continue
            # 공지 스킵
            first = tds[0].get_text(strip=True)
            if "공지" in first or "이벤트" in first:
                continue

            a_tag = row.select_one("td.title a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue

            # 조회수: 마지막 td
            views = 0
            views_text = tds[-1].get_text(strip=True).replace(",", "")
            try:
                views = int(views_text)
            except ValueError:
                pass

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, views=views))

        return posts
