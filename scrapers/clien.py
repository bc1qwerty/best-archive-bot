import re

import httpx

from scrapers.base import BaseScraper, Post

_K_RE = re.compile(r"([\d.]+)\s*k", re.IGNORECASE)


class ClienScraper(BaseScraper):
    community = "clien"
    community_name = "클리앙"
    base_url = "https://www.clien.net"

    @staticmethod
    def _parse_hit(text: str) -> int:
        """조회수 파싱 ('13.8 k' → 13800, '607' → 607)"""
        text = text.strip().replace(",", "")
        m = _K_RE.match(text)
        if m:
            return int(float(m.group(1)) * 1000)
        try:
            return int(text)
        except ValueError:
            return 0

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/service/group/clien_all?&od=T33")
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

            votes = 0
            symph = item.select_one("div.list_symph span")
            if symph:
                try:
                    votes = int(symph.get_text(strip=True))
                except ValueError:
                    pass

            views = 0
            hit_span = item.select_one("div.list_hit span.hit")
            if hit_span:
                views = self._parse_hit(hit_span.get_text(strip=True))

            comments = 0
            reply_span = item.select_one("a.list_reply span")
            if reply_span:
                try:
                    comments = int(reply_span.get_text(strip=True))
                except ValueError:
                    pass

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, votes=votes, views=views,
                                         comments=comments))

        return posts
