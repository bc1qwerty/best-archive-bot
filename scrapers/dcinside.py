import httpx

from scrapers.base import BaseScraper, Post


class DcinsideScraper(BaseScraper):
    community = "dcinside"
    community_name = "DC인사이드"
    base_url = "https://gall.dcinside.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/board/lists/?id=dcbest")
        posts = []

        for row in soup.select("tr.ub-content"):
            a_tag = row.select_one("td.gall_tit a:not(.reply_numbox)")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue

            votes = 0
            rec_td = row.select_one("td.gall_recommend")
            if rec_td:
                try:
                    votes = int(rec_td.get_text(strip=True))
                except ValueError:
                    pass

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, votes))

        return posts
