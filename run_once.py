"""GitHub Actions용 one-shot 실행 스크립트.

1) DB 초기화 + 오래된 레코드 정리
2) 전체 스크래퍼 병렬 실행
3) 미전송분 필터링 → Telegram 순차 전송
4) 종료
"""

import asyncio
import logging
import random
import sys
from collections import defaultdict

import httpx
from telegram import Bot, LinkPreviewOptions
from telegram.error import RetryAfter

from config.settings import BOT_TOKEN, CHAT_ID
from bot.formatter import format_single_post
from scrapers.schedule import SCRAPER_SCHEDULE
from db.database import post_db
from scrapers.base import Post

logger = logging.getLogger(__name__)

SEND_DELAY = 3.0       # 전송 간격 (초) — 분당 ~20건
MAX_SEND_PER_RUN = 10   # 1회 최대 전송 건수
MAX_SEND_RETRY = 2      # 전송 실패 재시도 횟수
SCRAPE_TIMEOUT = 60      # 스크래퍼 개별 타임아웃 (초)


def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )
    logging.getLogger("httpx").setLevel(logging.WARNING)


async def scrape_all() -> list[Post]:
    """전체 스크래퍼 병렬 실행, 수집된 게시글 반환."""
    all_posts: list[Post] = []

    async with httpx.AsyncClient(verify=False, follow_redirects=True) as client:

        async def _run_one(scraper, _interval, _delay):
            name = scraper.community_name
            try:
                posts = await asyncio.wait_for(
                    scraper.safe_fetch(client), timeout=SCRAPE_TIMEOUT,
                )
                logger.info("[%s] %d건 수집", name, len(posts))
                return posts
            except asyncio.TimeoutError:
                logger.error("[%s] 타임아웃 (%ds)", name, SCRAPE_TIMEOUT)
                return []
            except Exception:
                logger.exception("[%s] 스크래핑 실패", name)
                return []

        results = await asyncio.gather(
            *[_run_one(s, i, d) for s, i, d in SCRAPER_SCHEDULE],
        )

    for posts in results:
        all_posts.extend(posts)

    logger.info("총 %d건 수집 완료", len(all_posts))
    return all_posts


def interleave_posts(posts: list[Post]) -> list[Post]:
    """커뮤니티별 라운드로빈으로 게시글을 섞어 같은 커뮤니티가 연속되지 않도록 한다."""
    if len(posts) <= 1:
        return posts

    # 커뮤니티별 그룹핑
    by_community: dict[str, list[Post]] = defaultdict(list)
    for p in posts:
        by_community[p.community].append(p)

    # 각 커뮤니티 내부 순서를 랜덤으로 섞기
    for group in by_community.values():
        random.shuffle(group)

    # 글이 많은 커뮤니티 순으로 정렬 (균등 분산 효과)
    buckets = sorted(by_community.values(), key=len, reverse=True)

    # 라운드로빈: 각 버킷에서 1개씩 순서대로 꺼냄
    result: list[Post] = []
    idx = 0
    while buckets:
        # 아직 남은 버킷만 유지
        buckets = [b for b in buckets if b]
        if not buckets:
            break
        bucket = buckets[idx % len(buckets)]
        result.append(bucket.pop(0))
        idx += 1

    logger.info("큐 인터리빙 완료: %d건 (%d개 커뮤니티)", len(result), len(by_community))
    return result


async def send_posts(bot: Bot, posts: list[Post]) -> int:
    """미전송 게시글을 순차 전송, 성공 건수 반환."""
    if not posts:
        logger.info("전송할 게시글 없음")
        return 0

    # MAX_SEND_PER_RUN 제한
    if len(posts) > MAX_SEND_PER_RUN:
        logger.warning("전송 대상 %d건 → %d건으로 제한", len(posts), MAX_SEND_PER_RUN)
        posts = posts[:MAX_SEND_PER_RUN]

    sent_count = 0
    for post in posts:
        msg = format_single_post(post)
        success = False

        for attempt in range(1, MAX_SEND_RETRY + 1):
            try:
                await bot.send_message(
                    chat_id=CHAT_ID,
                    text=msg,
                    disable_notification=True,
                    link_preview_options=LinkPreviewOptions(
                        prefer_large_media=True,
                        show_above_text=False,
                    ),
                )
                success = True
                break
            except RetryAfter as e:
                logger.warning("Rate limit, %d초 대기", e.retry_after)
                await asyncio.sleep(e.retry_after + 1)
            except Exception:
                logger.warning(
                    "[%s] 전송 실패 (%d/%d): %s",
                    post.community_name, attempt, MAX_SEND_RETRY, post.title,
                )
                if attempt < MAX_SEND_RETRY:
                    await asyncio.sleep(SEND_DELAY)

        if success:
            # DB 기록 실패 시 재시도 (실패하면 다음 실행에서 중복 발송됨)
            for db_attempt in range(1, 4):
                try:
                    await post_db.mark_sent([post])
                    break
                except Exception:
                    logger.warning(
                        "[%s] mark_sent 실패 (%d/3): %s",
                        post.community_name, db_attempt, post.url,
                    )
                    if db_attempt < 3:
                        await asyncio.sleep(1)
            sent_count += 1
            await asyncio.sleep(SEND_DELAY)
        else:
            logger.error("[%s] 전송 최종 실패: %s", post.community_name, post.title)

    return sent_count


async def main():
    setup_logging()
    logger.info("=== run_once 시작 ===")

    if not BOT_TOKEN or BOT_TOKEN == "your_bot_token_here":
        logger.error("BOT_TOKEN이 설정되지 않음")
        sys.exit(1)
    if not CHAT_ID or CHAT_ID == "your_chat_id_here":
        logger.error("CHAT_ID가 설정되지 않음")
        sys.exit(1)

    # DB 초기화 + 정리
    await post_db.init()
    await post_db.cleanup_old_records()

    # 스크래핑
    all_posts = await scrape_all()

    # 미전송분 필터링 → 커뮤니티 인터리빙
    unsent = await post_db.filter_unsent(all_posts)
    logger.info("미전송 게시글: %d건", len(unsent))
    unsent = interleave_posts(unsent)

    # 전송
    bot = Bot(token=BOT_TOKEN)
    async with bot:
        sent = await send_posts(bot, unsent)

    logger.info("=== run_once 완료: %d건 전송 ===", sent)


if __name__ == "__main__":
    asyncio.run(main())
