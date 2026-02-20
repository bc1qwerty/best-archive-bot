import httpx

from scrapers.base import BaseScraper, Post


class HumorunivScraper(BaseScraper):
    community = "humoruniv"
    community_name = "웃긴대학"
    base_url = "http://web.humoruniv.com"
    encoding = "euc-kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/board/humor/board_best.html")
        posts = []

        for row in soup.select("table.list_table tr"):
            a_tag = row.select_one("a[href*='read.html']")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue

            if href.startswith("http"):
                url = href
            elif href.startswith("/"):
                url = f"{self.base_url}{href}"
            else:
                # 상대 경로 (같은 디렉토리)
                url = f"{self.base_url}/board/humor/{href}"
            posts.append(self._make_post(title, url))

        # 대체: 좀 더 넓은 셀렉터
        if not posts:
            seen = set()
            for a_tag in soup.find_all("a", href=True):
                href = a_tag["href"]
                if "read.html" not in href:
                    continue
                title = a_tag.get_text(strip=True)
                if not title or len(title) < 2:
                    continue
                if href.startswith("http"):
                    url = href
                elif href.startswith("/"):
                    url = f"{self.base_url}{href}"
                else:
                    url = f"{self.base_url}/board/humor/{href}"
                if url not in seen:
                    seen.add(url)
                    posts.append(self._make_post(title, url))

        return posts
