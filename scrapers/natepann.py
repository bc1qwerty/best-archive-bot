import re

import httpx

from scrapers.base import BaseScraper, Post


class NatepannScraper(BaseScraper):
    community = "natepann"
    community_name = "네이트판"
    base_url = "https://pann.nate.com"
    data_required = True

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/talk/ranking")
        posts = []

        for li in soup.select("ul.post_wrap > li"):
            a_tag = li.select_one("h2 a[href]")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not title or not href:
                continue

            votes = 0
            rcm = li.select_one("span.rcm")
            if rcm:
                m = re.search(r"\d+", rcm.get_text())
                if m:
                    votes = int(m.group())

            comments = 0
            reple = li.select_one("span.reple-num")
            if reple:
                m = re.search(r"\d+", reple.get_text())
                if m:
                    comments = int(m.group())

            url = f"{self.base_url}{href}" if not href.startswith("http") else href
            posts.append(self._make_post(title, url, votes, comments=comments))

        # 기존 방식 폴백
        if not posts:
            for a_tag in soup.find_all("a", href=re.compile(r"^/talk/\d+")):
                if a_tag.parent and a_tag.parent.name == "h2":
                    title = a_tag.get_text(strip=True)
                    href = a_tag["href"]
                    if not title:
                        continue
                    url = f"{self.base_url}{href}"
                    posts.append(self._make_post(title, url))

        return posts
