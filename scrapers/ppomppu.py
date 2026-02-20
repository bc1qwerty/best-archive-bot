import re

import httpx

from scrapers.base import BaseScraper, Post


class PpomppuScraper(BaseScraper):
    community = "ppomppu"
    community_name = "뽐뿌"
    base_url = "https://www.ppomppu.co.kr"
    encoding = "euc-kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/hot.php")
        posts = []

        for row in soup.select("tr.baseList.bbs_new1"):
            # 광고/업체 스킵
            tds_all = row.find_all("td")
            if tds_all:
                cat = tds_all[0].get_text(strip=True)
                if cat in ("렌탈업체", "뽐뿌스폰서", "가전견적상담", "휴대폰업체"):
                    continue

            a_tag = row.select_one(".baseList-title a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue

            # AD 텍스트 스킵
            if title.startswith("AD"):
                continue

            # 추천수: "N - N" 패턴 (추천 - 비추)
            votes = 0
            views = 0
            tds = row.find_all("td", class_="baseList-space")
            for td in tds:
                text = td.get_text(strip=True)
                m = re.match(r"(\d+)\s*-\s*\d+", text)
                if m:
                    votes = int(m.group(1))
                    break

            # 조회수: 마지막 td (숫자만)
            if tds:
                views_text = tds[-1].get_text(strip=True).replace(",", "")
                try:
                    views = int(views_text)
                except ValueError:
                    pass

            if href.startswith("http"):
                url = href
            elif href.startswith("/"):
                url = f"{self.base_url}{href}"
            else:
                url = f"{self.base_url}/{href}"
            posts.append(self._make_post(title, url, votes=votes, views=views))

        # 중복 URL 제거
        seen = set()
        unique = []
        for p in posts:
            if p.url not in seen:
                seen.add(p.url)
                unique.append(p)

        return unique
