import asyncio
import json
import logging
from collections import deque
from dataclasses import asdict
from datetime import datetime, timezone, timedelta
from pathlib import Path

import httpx
from telegram import Bot, LinkPreviewOptions
from telegram.error import RetryAfter
from telegram.ext import Application, ContextTypes

from bot.admin_commands import register_admin_commands
from bot.formatter import format_single_post
from config.settings import ADMIN_ID, BASE_DIR, BOT_TOKEN, CHAT_ID
from db.database import post_db
from scrapers.base import BaseScraper, Post
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
from scrapers.etoland import EtolandScraper
from scrapers.dvdprime import DvdprimeScraper

logger = logging.getLogger(__name__)

KST = timezone(timedelta(hours=9))
QUEUE_BACKUP_PATH = BASE_DIR / "data" / "send_queue.json"

# (스크래퍼, 주기(분), 첫 실행 지연(초))
SCRAPER_SCHEDULE: list[tuple[BaseScraper, int, int]] = [
    # --- 실시간 베스트 (10분) ---
    (DcinsideScraper(),  10,  10),
    (TheqooScraper(),    10,  50),
    (NatepannScraper(),  10,  90),

    # --- 일반 인기글 (15분) ---
    (ClienScraper(),     15, 130),
    (BobaedreamScraper(),15, 170),
    (MlbparkScraper(),   15, 210),
    (PpomppuScraper(),   15, 250),

    # --- 교체 느린 사이트 (20분) ---
    (RuliwebScraper(),   20, 290),
    (InvenScraper(),     20, 330),
    (Cook82Scraper(),    20, 370),
    (HumorunivScraper(), 20, 410),

    # --- 봇 차단 주의 (30분) ---
    (FmkoreaScraper(),   30, 450),
    (EtolandScraper(),   20, 490),
    (DvdprimeScraper(),  20, 530),
]

# 전역 전송 큐
_send_queue: deque[Post] = deque()
_send_retry: dict[str, int] = {}  # url → 재시도 횟수
MAX_SEND_RETRY = 3

# 연속 실패 카운터 / 마지막 성공 시각 / 백오프
_fail_count: dict[str, int] = {}
_last_success: dict[str, datetime] = {}
_backoff_until: dict[str, datetime] = {}  # 커뮤니티별 백오프 만료 시각
FAIL_ALERT_THRESHOLD = 3
HEALTH_CHECK_MINUTES = 30
SEND_INTERVAL = 5
_last_queue_len: int = 0  # 직전 백업 시 큐 길이


# ── 큐 분산 삽입 ──

def _interleave_into_queue(new_posts: list[Post]):
    """새 게시글을 기존 큐에 균등 분산 삽입"""
    if not new_posts:
        return
    if not _send_queue:
        _send_queue.extend(new_posts)
        return

    # 기존 큐를 리스트로 변환 후 균등 간격으로 삽입
    items = list(_send_queue)
    step = max(1, len(items) // len(new_posts) + 1)
    for i, post in enumerate(new_posts):
        pos = min((i + 1) * step, len(items))
        items.insert(pos, post)

    _send_queue.clear()
    _send_queue.extend(items)


# ── 큐 백업/복원 ──

def _save_queue():
    """큐를 JSON 파일로 백업"""
    try:
        data = [asdict(p) for p in _send_queue]
        QUEUE_BACKUP_PATH.write_text(json.dumps(data, ensure_ascii=False), encoding="utf-8")
    except Exception:
        logger.warning("큐 백업 실패")


def _load_queue():
    """시작 시 백업된 큐 복원"""
    if not QUEUE_BACKUP_PATH.exists():
        return
    try:
        data = json.loads(QUEUE_BACKUP_PATH.read_text(encoding="utf-8"))
        for item in data:
            _send_queue.append(Post(**item))
        QUEUE_BACKUP_PATH.unlink()
        if data:
            logger.info("큐 복원: %d건", len(data))
    except Exception:
        logger.warning("큐 복원 실패")


# ── 전송 ──

async def _send_with_retry(bot: Bot, chat_id, text, **kwargs):
    """RetryAfter 대응 전송"""
    try:
        await bot.send_message(chat_id=chat_id, text=text, **kwargs)
    except RetryAfter as e:
        logger.warning("Rate limit, %d초 대기", e.retry_after)
        await asyncio.sleep(e.retry_after + 1)
        await bot.send_message(chat_id=chat_id, text=text, **kwargs)


async def _notify_admin(bot: Bot, text: str):
    """관리자 DM 알림"""
    try:
        await bot.send_message(chat_id=ADMIN_ID, text=text)
    except Exception:
        logger.warning("관리자 알림 전송 실패")


async def sender_job(context: ContextTypes.DEFAULT_TYPE):
    """큐에서 1건씩 꺼내 전송, 실패 시 최대 3회 재삽입"""
    if not _send_queue:
        return

    post = _send_queue.popleft()
    msg = format_single_post(post)

    try:
        await _send_with_retry(
            context.bot,
            chat_id=CHAT_ID,
            text=msg,
            link_preview_options=LinkPreviewOptions(
                prefer_large_media=True,
                show_above_text=False,
            ),
        )
        _send_retry.pop(post.url, None)
        try:
            await post_db.mark_sent([post])
        except Exception:
            logger.warning("[%s] DB mark_sent 오류", post.community_name)
    except Exception:
        retries = _send_retry.get(post.url, 0) + 1
        if retries < MAX_SEND_RETRY:
            _send_retry[post.url] = retries
            _send_queue.append(post)
            logger.warning("[%s] 전송 실패, 재시도 %d/%d: %s",
                           post.community_name, retries, MAX_SEND_RETRY, post.title)
        else:
            _send_retry.pop(post.url, None)
            logger.error("[%s] 전송 최종 실패, 폐기: %s", post.community_name, post.title)


# ── 스크래핑 ──

async def scrape_one_community(context: ContextTypes.DEFAULT_TYPE):
    """단일 커뮤니티 스크래핑 → 큐에 적재"""
    scraper: BaseScraper = context.job.data
    is_retry = context.job.name.startswith("retry_")
    name = scraper.community_name
    key = scraper.community

    # 백오프 중이면 스킵
    if key in _backoff_until:
        if datetime.now(KST) < _backoff_until[key]:
            logger.info("[%s] 백오프 중, 스킵", name)
            return
        del _backoff_until[key]

    logger.info("[%s] 수집 시작", name)

    # 개별 페이지 크롤링 스크래퍼는 타임아웃 확대
    timeout = 120 if key in ("mlbpark", "cook82") else 30

    try:
        client = context.application.bot_data.get("http_client")
        if client is None or client.is_closed:
            client = httpx.AsyncClient(verify=False, follow_redirects=True)
            context.application.bot_data["http_client"] = client
        posts = await asyncio.wait_for(
            scraper.safe_fetch(client),
            timeout=timeout,
        )
    except asyncio.TimeoutError:
        logger.error("[%s] 타임아웃", name)
        await _handle_failure(context, scraper, is_retry, "타임아웃")
        return
    except httpx.HTTPStatusError as e:
        code = e.response.status_code
        if code in (403, 429):
            minutes = 30 if code == 403 else 10
            _backoff_until[key] = datetime.now(KST) + timedelta(minutes=minutes)
            logger.warning("[%s] HTTP %d → %d분 백오프", name, code, minutes)
            await _notify_admin(
                context.bot,
                f"⚠️ [{name}] HTTP {code} → {minutes}분 백오프",
            )
            return
        logger.exception("[%s] HTTP 오류", name)
        await _handle_failure(context, scraper, is_retry, f"HTTP {code}")
        return
    except Exception as e:
        logger.exception("[%s] 스크래핑 오류", name)
        await _handle_failure(context, scraper, is_retry, str(e)[:100])
        return

    if not posts:
        await _handle_failure(context, scraper, is_retry, "게시글 0건")
        return

    _fail_count[key] = 0
    _last_success[key] = datetime.now(KST)

    try:
        unsent = await post_db.filter_unsent(posts)
    except Exception:
        logger.exception("[%s] DB 오류", name)
        return

    if not unsent:
        logger.info("[%s] 새 게시글 없음", name)
        return

    _interleave_into_queue(unsent)
    logger.info("[%s] %d개 큐 적재 (큐 대기: %d)", name, len(unsent), len(_send_queue))


async def _handle_failure(context, scraper, is_retry, reason):
    """실패 처리: 재시도 스케줄 + 연속 실패 알림"""
    key = scraper.community
    name = scraper.community_name
    _fail_count[key] = _fail_count.get(key, 0) + 1

    if not is_retry:
        context.application.job_queue.run_once(
            scrape_one_community,
            when=30,
            data=scraper,
            name=f"retry_{key}",
        )
        logger.info("[%s] 30초 후 재시도 예약", name)
        return

    count = _fail_count[key]
    if count >= FAIL_ALERT_THRESHOLD and count % FAIL_ALERT_THRESHOLD == 0:
        await _notify_admin(
            context.bot,
            f"⚠️ [{name}] {count}회 연속 실패\n사유: {reason}",
        )


# ── 헬스체크 / 정리 ──

async def health_check_job(context: ContextTypes.DEFAULT_TYPE):
    """전체 스크래핑 헬스체크"""
    if not _last_success:
        return

    now = datetime.now(KST)
    threshold = timedelta(minutes=HEALTH_CHECK_MINUTES)

    all_stale = all(
        (now - ts) > threshold for ts in _last_success.values()
    )
    if all_stale:
        oldest = min(_last_success.values())
        elapsed = int((now - oldest).total_seconds() / 60)
        await _notify_admin(
            context.bot,
            f"🚨 전체 스크래핑 {elapsed}분째 수집 없음!",
        )


async def queue_backup_job(context: ContextTypes.DEFAULT_TYPE):
    """큐 주기적 백업 (크래시 대비) — 변화 없으면 스킵"""
    global _last_queue_len
    cur_len = len(_send_queue)

    # 큐 비었고 백업 파일도 없으면 스킵
    if cur_len == 0 and not QUEUE_BACKUP_PATH.exists():
        return

    # 큐 길이가 직전 백업과 동일하면 스킵
    if cur_len == _last_queue_len:
        return

    _save_queue()
    _last_queue_len = cur_len


async def cleanup_job(context: ContextTypes.DEFAULT_TYPE):
    """매시간 오래된 DB 레코드 정리"""
    try:
        await post_db.cleanup_old_records()
    except Exception:
        logger.warning("DB cleanup 오류")


# ── 앱 생성 ──

def create_application() -> Application:
    """텔레그램 봇 Application 생성"""
    # 백업된 큐 복원
    _load_queue()

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

    # 전송 큐 소비: 5초마다 1건
    job_queue.run_repeating(sender_job, interval=SEND_INTERVAL, first=15, name="sender")

    # 관리 명령어 등록
    register_admin_commands(app)

    # DB 정리: 매시간
    job_queue.run_repeating(cleanup_job, interval=3600, first=3600, name="cleanup")

    # 큐 백업: 5분마다 (크래시 대비)
    job_queue.run_repeating(queue_backup_job, interval=300, first=300, name="queue_backup")

    # 헬스체크: 10분마다
    job_queue.run_repeating(health_check_job, interval=600, first=600, name="health_check")

    logger.info("봇 초기화 완료 (%d개 커뮤니티)", len(SCRAPER_SCHEDULE))
    return app
