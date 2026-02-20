import httpx

from scrapers.base import BaseScraper, Post


class BobaedreamScraper(BaseScraper):
    community = "bobaedream"
    community_name = "보배드림"
    base_url = "https://www.bobaedream.co.kr"

    async def fetch_best_posts(self, client: httpx.AsyncClient) -> list[Post]:
        soup = await self._fetch_html(client, f"{self.base_url}/list?code=best")
        posts = []

        table = soup.find("table", class_="clistTable02")
        if not table:
            return []

        for tr in table.find_all("tr"):
            a_tag = tr.select_one("td.pl14 a")
            if not a_tag:
                continue
            title = a_tag.get_text(strip=True)
            href = a_tag.get("href", "")
            if not href or not title:
                continue

            votes = 0
            rec_td = tr.select_one("td.recomm")
            if rec_td:
                try:
                    votes = int(rec_td.get_text(strip=True).replace(",", ""))
                except ValueError:
                    pass

            views = 0
            cnt_td = tr.select_one("td.count")
            if cnt_td:
                try:
                    views = int(cnt_td.get_text(strip=True).replace(",", ""))
                except ValueError:
                    pass

            comments = 0
            reply_span = tr.select_one("span.totreply")
            if reply_span:
                try:
                    comments = int(reply_span.get_text(strip=True))
                except ValueError:
                    pass

            url = href if href.startswith("http") else f"{self.base_url}{href}"
            posts.append(self._make_post(title, url, votes=votes, views=views,
                                         comments=comments))

        return posts
