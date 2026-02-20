import re

import httpx

from scrapers.base import BaseScraper, Post

_MAN_RE = re.compile(r"([\d.]+)만")


class InvenScraper(BaseScraper):
    community = "inven"
    community_name = "인벤"
    base_url = "https://www.inven.co.kr"

    @staticmethod
    def _parse_views(text: str) -> int:
        """조회수 파싱 ('1.1만' → 11000, '5,559' → 5559)"""
        text = text.strip().replace(",", "")
        m = _MAN_RE.match(text)
        if m:
            return int(float(m.group(1)) * 10000)
        try:
            return int(text)
        except ValueError:
            return 0

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/hot/")
        posts = []

        for item in soup.find_all("div", class_="list-common"):
            if "con" not in (item.get("class") or []):
                continue

            # 제목 링크
            title_div = item.find("div", class_="title")
            if not title_div:
                continue
            a_tag = title_div.find("a")
            if not a_tag:
                continue
            href = a_tag.get("href", "")
            if not href:
                continue

            # 제목: name에서 순번·카테고리 제거
            name_div = item.find("div", class_="name")
            if not name_div:
                continue
            title = name_div.get_text(strip=True)
            num_div = item.find("div", class_="num")
            cate_div = item.find("div", class_="cate")
            if num_div:
                num_text = num_div.get_text(strip=True)
                if title.startswith(num_text):
                    title = title[len(num_text):]
            if cate_div:
                cate_text = cate_div.get_text(strip=True)
                if title.startswith(cate_text):
                    title = title[len(cate_text):]
            title = title.strip()
            if not title:
                continue

            # 추천수
            votes = 0
            reco_div = item.find("div", class_="reco")
            if reco_div:
                try:
                    votes = int(reco_div.get_text(strip=True))
                except ValueError:
                    pass

            # 조회수
            views = 0
            hits_div = item.find("div", class_="hits")
            if hits_div:
                views = self._parse_views(hits_div.get_text(strip=True))

            # 댓글수
            comments = 0
            cmt_div = item.find("div", class_="comment")
            if cmt_div:
                m = re.search(r"\d+", cmt_div.get_text())
                if m:
                    comments = int(m.group())

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, votes=votes, views=views,
                                         comments=comments))

        return posts
