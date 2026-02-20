import asyncio
import re

import httpx

from scrapers.base import BaseScraper, Post
from utils.http_client import fetch_html

_NUM_RE = re.compile(r"(\d[\d,]*)")
_BO_RE = re.compile(r'g4_bo_table\s*=\s*"([^"]+)"')
_WR_RE = re.compile(r'bo_table=\w+&wr_id=(\d+)')


class EtolandScraper(BaseScraper):
    community = "etoland"
    community_name = "이토랜드"
    base_url = "https://www.etoland.co.kr"
    encoding = "euc-kr"

    async def _resolve_real_url(self, client: httpx.AsyncClient, hit_url: str) -> str | None:
        """hit.php?bn_id= 래퍼 URL → board.php 실제 URL 변환"""
        try:
            html = await fetch_html(client, hit_url, encoding=self.encoding, referer=self.base_url)
            bo = _BO_RE.search(html)
            wr = _WR_RE.search(html)
            if bo and wr:
                return f"{self.base_url}/bbs/board.php?bo_table={bo.group(1)}&wr_id={wr.group(1)}"
        except Exception:
            pass
        return None

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/bbs/hit.php")
        # 1단계: 히트 목록에서 기본 정보 수집
        raw_posts = []
        seen_bn = set()

        for li in soup.find_all("li", class_=lambda c: c and "hit_item" in c):
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

            # hit.php?bn_id= 형태 → 절대 URL
            if href.startswith("http"):
                hit_url = href
            elif href.startswith("/"):
                hit_url = f"{self.base_url}{href}"
            else:
                hit_url = f"{self.base_url}/bbs/{href}"

            if hit_url in seen_bn:
                continue
            seen_bn.add(hit_url)
            raw_posts.append((title, hit_url, votes, views, comments))

        # 2단계: hit.php?bn_id= URL → board.php 실제 URL 병렬 변환
        tasks = [self._resolve_real_url(client, hit_url) for _, hit_url, *_ in raw_posts]
        resolved = await asyncio.gather(*tasks)

        posts = []
        seen_url = set()
        for (title, hit_url, votes, views, comments), real_url in zip(raw_posts, resolved):
            url = real_url or hit_url  # 변환 실패 시 원본 유지
            if url not in seen_url:
                seen_url.add(url)
                posts.append(self._make_post(title, url, votes=votes, views=views,
                                             comments=comments))

        return posts
