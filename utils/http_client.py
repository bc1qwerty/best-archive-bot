import asyncio
import logging
import random

import httpx

from config.settings import MAX_CONCURRENT_REQUESTS, REQUEST_DELAY_MIN, REQUEST_DELAY_MAX

logger = logging.getLogger(__name__)

USER_AGENTS = [
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36 Edg/133.0.0.0",
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.3 Safari/605.1.15",
]

# 동시 요청 제한 세마포어
_semaphore = asyncio.Semaphore(MAX_CONCURRENT_REQUESTS)


async def fetch_html(
    client: httpx.AsyncClient,
    url: str,
    encoding: str = "utf-8",
    referer: str | None = None,
) -> str:
    """URL에서 HTML을 비동기로 가져오기"""
    headers = {
        "User-Agent": random.choice(USER_AGENTS),
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        "Accept-Language": "ko-KR,ko;q=0.9,en-US;q=0.8,en;q=0.7",
    }
    if referer:
        headers["Referer"] = referer

    async with _semaphore:
        await asyncio.sleep(random.uniform(REQUEST_DELAY_MIN, REQUEST_DELAY_MAX))
        resp = await client.get(url, headers=headers, timeout=15.0)
        resp.raise_for_status()
        if encoding != "utf-8":
            return resp.content.decode(encoding, errors="replace")
        return resp.text
