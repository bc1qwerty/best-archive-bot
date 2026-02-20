import re
from urllib.parse import urljoin

import httpx

from scrapers.base import BaseScraper, Post

_NUM_RE = re.compile(r"(\d[\d,]*)")

# 섹션 제목 키워드 → Post 필드 매핑
_METRIC_MAP = {
    "추천": "votes",
    "댓글": "comments",
    "조회": "views",
}


class DvdprimeScraper(BaseScraper):
    community = "dvdprime"
    community_name = "DVD프라임"
    base_url = "https://dvdprime.com"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        page_url = f"{self.base_url}/g2/bbs/board.php?bo_table=comm"
        soup = await self._fetch_html(client, page_url)
        posts = []
        seen = set()

        for box in soup.find_all("div", class_="bottom_row_third"):
            # 섹션 제목으로 메트릭 종류 판별 (최다 추천 / 최다 댓글 / 최다 조회)
            title_div = box.find("div", class_="bottom_row_title")
            if not title_div:
                continue
            section_text = title_div.get_text(strip=True)
            metric = None
            for keyword, field in _METRIC_MAP.items():
                if keyword in section_text:
                    metric = field
                    break
            if not metric:
                continue

            for row in box.find_all("div", class_="rc_box_list_row_inner"):
                right = row.find("div", class_="rc_box_best_right")
                if not right:
                    continue
                a_tag = right.find("a")
                if not a_tag:
                    continue
                title = a_tag.get_text(strip=True)
                href = a_tag.get("href", "")
                if not title or not href:
                    continue

                # 수치 (추천수/댓글수/조회수)
                count = 0
                right2 = row.find("div", class_="rc_box_best_right2")
                if right2:
                    m = _NUM_RE.search(right2.get_text(strip=True))
                    if m:
                        count = int(m.group(1).replace(",", ""))

                url = urljoin(page_url, href)

                if url not in seen:
                    seen.add(url)
                    posts.append(self._make_post(
                        title, url, **{metric: count},
                    ))

        return posts
