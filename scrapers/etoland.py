import re

import httpx

from scrapers.base import BaseScraper, Post

_NUM_RE = re.compile(r"(\d[\d,]*)")


class EtolandScraper(BaseScraper):
    community = "etoland"
    community_name = "이토랜드"
    base_url = "https://www.etoland.co.kr"
    encoding = "euc-kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/bbs/hit.php")
        posts = []
        seen = set()

        for li in soup.find_all("li", class_=lambda c: c and "hit_item" in c):
            # 광고 스킵
            cls = " ".join(li.get("class", []))
            if "ad_list" in cls:
                continue

            a_tag = li.find("a", class_="content_link")
            if not a_tag:
                continue
            subj = li.find("p", class_="subject")
            if not subj:
                continue
            title = subj.get_text(strip=True)
            href = a_tag.get("href", "")
            if not title or not href:
                continue

            # 추천수
            votes = 0
            good_span = li.find("span", class_="good")
            if good_span:
                m = _NUM_RE.search(good_span.get_text(strip=True))
                if m:
                    votes = int(m.group(1).replace(",", ""))

            # 조회수
            views = 0
            hit_span = li.find("span", class_="hit")
            if hit_span:
                m = _NUM_RE.search(hit_span.get_text(strip=True))
                if m:
                    views = int(m.group(1).replace(",", ""))

            # 댓글수
            comments = 0
            cmt_span = li.find("span", class_="comment_cnt")
            if cmt_span:
                m = _NUM_RE.search(cmt_span.get_text(strip=True))
                if m:
                    comments = int(m.group(1).replace(",", ""))

            if href.startswith("http"):
                url = href
            elif href.startswith("/"):
                url = f"{self.base_url}{href}"
            else:
                url = f"{self.base_url}/bbs/{href}"

            if url not in seen:
                seen.add(url)
                posts.append(self._make_post(title, url, votes=votes, views=views,
                                             comments=comments))

        return posts
