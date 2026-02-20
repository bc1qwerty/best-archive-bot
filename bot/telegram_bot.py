import asyncio
import logging

import httpx
from telegram import Bot, LinkPreviewOptions
from telegram.ext import Application, ContextTypes

from bot.admin_commands import register_admin_commands
from bot.formatter import format_single_post
from config.settings import BOT_TOKEN, CHAT_ID
from db.database import PostDatabase
from scrapers.base import BaseScraper
from scrapers.dcinside import DcinsideScraper
from scrapers.fmkorea import FmkoreaScraper
from scrapers.clien import ClienScraper
from scrapers.ppomppu import PpomppuScraper
from scrapers.ruliweb import RuliwebScraper
from scrapers.inven import InvenScraper
from scrapers.humoruniv import HumorunivScraper
from scrapers.theqoo import TheqooScraper
from scrapers.natepann import NatepannScraper
from scrapers.bobaedream import BobaedreamScraper
from scrapers.cook82 import Cook82Scraper
from scrapers.mlbpark import MlbparkScraper

logger = logging.getLogger(__name__)

# (스크래퍼, 주기(분), 첫 실행 지연(초))
# 실시간 베스트 (교체 빠름)  → 10분
# 일반 인기글                → 15분
# 교체 느린 사이트 / 소규모  → 20분
# 봇 차단 이력               → 30분
SCRAPER_SCHEDULE: list[tuple[BaseScraper, int, int]] = [
    # --- 실시간 베스트 (10분) ---
    (DcinsideScraper(),  10,  10),   # DC 실베, 교체 매우 빠름
    (TheqooScraper(),    10,  50),   # 실시간 핫글
    (NatepannScraper(),  10,  90),   # 실시간 랭킹

    # --- 일반 인기글 (15분) ---
    (ClienScraper(),     15, 130),
    (BobaedreamScraper(),15, 170),
    (MlbparkScraper(),   15, 210),
    (PpomppuScraper(),   15, 250),

    # --- 교체 느린 사이트 (20분) ---
    (RuliwebScraper(),   20, 290),   # 베스트 선정, 느림
    (InvenScraper(),     20, 330),
    (Cook82Scraper(),    20, 370),   # 목록 10개뿐
    (HumorunivScraper(), 20, 410),

    # --- 봇 차단 주의 (30분) ---
    (FmkoreaScraper(),   30, 450),   # 430 차단 빈번
]


async def scrape_one_community(context: ContextTypes.DEFAULT_TYPE):
    """단일 커뮤니티 스크래핑 + 전송"""
    scraper: BaseScraper = context.job.data
    db = PostDatabase()
    bot: Bot = context.bot

    logger.info("[%s] 수집 시작", scraper.community_name)

    try:
        async with httpx.AsyncClient(verify=False, follow_redirects=True) as client:
            posts = await asyncio.wait_for(
                scraper.safe_fetch(client),
                timeout=30,
            )
    except asyncio.TimeoutError:
        logger.error("[%s] 타임아웃", scraper.community_name)
        return
    except Exception:
        logger.exception("[%s] 스크래핑 오류", scraper.community_name)
        return

    if not posts:
        return

    try:
        unsent = await db.filter_unsent(posts)
    except Exception:
        logger.exception("[%s] DB 오류", scraper.community_name)
        return

    if not unsent:
        logger.info("[%s] 새 게시글 없음", scraper.community_name)
        return

    sent_count = 0
    for post in unsent:
        msg = format_single_post(post)
        try:
            await bot.send_message(
                chat_id=CHAT_ID,
                text=msg,
                link_preview_options=LinkPreviewOptions(
                    prefer_large_media=True,
                    show_above_text=False,
                ),
            )
            try:
                await db.mark_sent([post])
            except Exception:
                logger.warning("[%s] DB mark_sent 오류", scraper.community_name)
            sent_count += 1
            await asyncio.sleep(3)
        except Exception:
            logger.exception("[%s] 전송 실패: %s", scraper.community_name, post.title)

    logger.info("[%s] %d개 전송 완료", scraper.community_name, sent_count)


async def cleanup_job(context: ContextTypes.DEFAULT_TYPE):
    """매시간 오래된 DB 레코드 정리"""
    try:
        db = PostDatabase()
        await db.cleanup_old_records()
    except Exception:
        logger.warning("DB cleanup 오류")


def create_application() -> Application:
    """텔레그램 봇 Application 생성"""
    app = Application.builder().token(BOT_TOKEN).build()
    job_queue = app.job_queue

    for scraper, interval_min, first_sec in SCRAPER_SCHEDULE:
        job_queue.run_repeating(
            scrape_one_community,
            interval=interval_min * 60,
            first=first_sec,
            data=scraper,
            name=f"scrape_{scraper.community}",
        )
        logger.info(
            "  [%s] %d분 주기, 첫 실행 %d초 후",
            scraper.community_name, interval_min, first_sec,
        )

    # 관리 명령어 등록
    register_admin_commands(app)

    # DB 정리: 매시간
    job_queue.run_repeating(cleanup_job, interval=3600, first=3600, name="cleanup")

    logger.info("봇 초기화 완료 (%d개 커뮤니티)", len(SCRAPER_SCHEDULE))
    return app
