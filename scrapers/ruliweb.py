import re

import httpx

from scrapers.base import BaseScraper, Post


class RuliwebScraper(BaseScraper):
    community = "ruliweb"
    community_name = "루리웹"
    base_url = "https://bbs.ruliweb.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/best/selection")
        posts = []

        for row in soup.select("tr.table_body"):
            a_tag = row.select_one("a.deco")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href:
                continue

            votes = 0
            rec_td = row.select_one("td.recomd")
            if rec_td:
                try:
                    votes = int(rec_td.get_text(strip=True))
                except ValueError:
                    pass

            comments = 0
            reply_span = row.select_one("span.num_reply")
            if reply_span:
                m = re.search(r"\d+", reply_span.get_text())
                if m:
                    comments = int(m.group())

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, votes, comments=comments))

        return posts
