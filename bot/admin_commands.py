import logging
from datetime import datetime, timedelta, timezone
from functools import wraps

from telegram import BotCommand, Update
from telegram.ext import Application, CommandHandler, ContextTypes

from config.settings import ADMIN_ID
from db.database import post_db

logger = logging.getLogger(__name__)

# 커뮤니티 영문키 → 한글 이름 매핑 (런타임에 채움)
_community_names: dict[str, str] = {}


def admin_only(func):
    """관리자 전용 데코레이터"""
    @wraps(func)
    async def wrapper(update: Update, context: ContextTypes.DEFAULT_TYPE):
        if update.effective_user.id != ADMIN_ID:
            return
        return await func(update, context)
    return wrapper


def _is_scraping_active(context: ContextTypes.DEFAULT_TYPE) -> bool:
    """스크래핑 job 존재 여부 확인"""
    jobs = context.application.job_queue.jobs()
    return any(j.name.startswith("scrape_") for j in jobs)


def _stop_scraping(context: ContextTypes.DEFAULT_TYPE, target: str | None = None) -> int:
    """스크래핑 job 제거. target 지정 시 해당 커뮤니티만."""
    jobs = context.application.job_queue.jobs()
    count = 0
    for job in jobs:
        if not job.name.startswith("scrape_"):
            continue
        if target and job.name != f"scrape_{target}":
            continue
        job.schedule_removal()
        count += 1
    return count


def _start_scraping(context: ContextTypes.DEFAULT_TYPE, target: str | None = None) -> int:
    """스크래핑 job 등록. target 지정 시 해당 커뮤니티만."""
    from bot.telegram_bot import scrape_one_community, SCRAPER_SCHEDULE
    job_queue = context.application.job_queue

    count = 0
    for scraper, interval_min, first_sec in SCRAPER_SCHEDULE:
        if target and scraper.community != target:
            continue
        job_queue.run_repeating(
            scrape_one_community,
            interval=interval_min * 60,
            first=first_sec,
            data=scraper,
            name=f"scrape_{scraper.community}",
        )
        count += 1
    return count


def _get_community_key(arg: str) -> str | None:
    """입력값으로 커뮤니티 키 찾기 (영문키 or 한글이름)"""
    if not arg:
        return None
    arg_lower = arg.lower()
    if arg_lower in _community_names:
        return arg_lower
    # 한글 이름으로 검색
    for key, name in _community_names.items():
        if arg == name:
            return key
    return None


@admin_only
async def cmd_status(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 상태 조회"""
    jobs = context.application.job_queue.jobs()
    scrape_jobs = [j for j in jobs if j.name.startswith("scrape_")]
    if scrape_jobs:
        names = ", ".join(
            _community_names.get(j.name.replace("scrape_", ""), j.name)
            for j in scrape_jobs
        )
        await update.message.reply_text(
            f"✅ 스크래핑 작동 중 ({len(scrape_jobs)}개)\n{names}"
        )
    else:
        await update.message.reply_text("⏹ 스크래핑 중지 상태")


@admin_only
async def cmd_stop(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 중지 (전체 또는 개별)"""
    arg = context.args[0] if context.args else None
    target = _get_community_key(arg) if arg else None

    if arg and not target:
        await update.message.reply_text(f"❌ '{arg}' 커뮤니티를 찾을 수 없습니다.")
        return

    if target:
        name = _community_names.get(target, target)
        count = _stop_scraping(context, target)
        if count == 0:
            await update.message.reply_text(f"⏹ [{name}] 이미 중지 상태입니다.")
        else:
            await update.message.reply_text(f"⏹ [{name}] 중지 완료")
            logger.info("관리자 명령: [%s] 중지", name)
    else:
        if not _is_scraping_active(context):
            await update.message.reply_text("⏹ 이미 중지 상태입니다.")
            return
        count = _stop_scraping(context)
        await update.message.reply_text(f"⏹ 스크래핑 중지 완료 ({count}개 job 제거)")
        logger.info("관리자 명령: 스크래핑 전체 중지 (%d개)", count)


@admin_only
async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 시작 (전체 또는 개별)"""
    arg = context.args[0] if context.args else None
    target = _get_community_key(arg) if arg else None

    if arg and not target:
        await update.message.reply_text(f"❌ '{arg}' 커뮤니티를 찾을 수 없습니다.")
        return

    if target:
        name = _community_names.get(target, target)
        # 이미 돌고 있는지 체크
        jobs = context.application.job_queue.jobs()
        already = any(j.name == f"scrape_{target}" for j in jobs)
        if already:
            await update.message.reply_text(f"✅ [{name}] 이미 작동 중입니다.")
            return
        _start_scraping(context, target)
        await update.message.reply_text(f"▶ [{name}] 시작")
        logger.info("관리자 명령: [%s] 시작", name)
    else:
        if _is_scraping_active(context):
            await update.message.reply_text("✅ 이미 작동 중입니다.")
            return
        count = _start_scraping(context)
        await update.message.reply_text(f"▶ 스크래핑 시작 ({count}개 job 등록)")
        logger.info("관리자 명령: 스크래핑 전체 시작 (%d개)", count)


@admin_only
async def cmd_restart(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 재시작"""
    _stop_scraping(context)
    count = _start_scraping(context)
    await update.message.reply_text(f"🔄 스크래핑 재시작 완료 ({count}개 job 등록)")
    logger.info("관리자 명령: 스크래핑 재시작 (%d개)", count)


@admin_only
async def cmd_stats(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """오늘 커뮤니티별 전송 통계"""
    rows = await post_db.get_today_stats()
    if not rows:
        await update.message.reply_text("📊 오늘 전송 기록 없음")
        return
    total = sum(count for _, count in rows)
    lines = [f"📊 <b>오늘 전송 통계</b> (총 {total}건)\n"]
    for community, count in rows:
        name = _community_names.get(community, community)
        lines.append(f"  {name}: {count}건")
    await update.message.reply_text("\n".join(lines), parse_mode="HTML")


@admin_only
async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """명령어 목록"""
    text = (
        "🤖 <b>BestArchiveBot 관리 명령어</b>\n\n"
        "/status — 스크래핑 상태 조회\n"
        "/start — 스크래핑 시작 (전체)\n"
        "/start &lt;커뮤니티&gt; — 개별 시작\n"
        "/stop — 스크래핑 중지 (전체)\n"
        "/stop &lt;커뮤니티&gt; — 개별 중지\n"
        "/restart — 스크래핑 재시작\n"
        "/stats — 오늘 전송 통계\n"
        "/help — 명령어 목록"
    )
    await update.message.reply_text(text, parse_mode="HTML")


async def _on_startup(app: Application) -> None:
    """봇 시작 시 메뉴 등록 + 알림"""
    try:
        commands = [
            BotCommand("status", "스크래핑 상태 조회"),
            BotCommand("start", "스크래핑 시작"),
            BotCommand("stop", "스크래핑 중지"),
            BotCommand("restart", "스크래핑 재시작"),
            BotCommand("stats", "오늘 전송 통계"),
            BotCommand("help", "명령어 목록"),
        ]
        await app.bot.set_my_commands(commands)
        now = datetime.now(timezone(timedelta(hours=9))).strftime("%Y-%m-%d %H:%M:%S")
        await app.bot.send_message(chat_id=ADMIN_ID, text=f"▶ BestArchiveBot 시작됨\n{now}")
        logger.info("봇 시작 알림 전송")
    except Exception:
        logger.warning("봇 시작 알림 전송 실패 (ADMIN_ID=%d)", ADMIN_ID)


async def _on_shutdown(app: Application) -> None:
    """봇 종료 시 큐 백업 + httpx 클라이언트 정리 + 알림"""
    from bot.telegram_bot import _save_queue
    _save_queue()
    # 공유 httpx 클라이언트 정리
    client = app.bot_data.get("http_client")
    if client and not client.is_closed:
        await client.aclose()
    try:
        await app.bot.send_message(chat_id=ADMIN_ID, text="⏹ BestArchiveBot 종료됨")
        logger.info("봇 종료 알림 전송")
    except Exception:
        logger.warning("봇 종료 알림 전송 실패")


def register_admin_commands(app: Application) -> None:
    """관리 명령어 핸들러 등록"""
    from bot.telegram_bot import SCRAPER_SCHEDULE

    # 커뮤니티 이름 매핑 구축
    for scraper, _, _ in SCRAPER_SCHEDULE:
        _community_names[scraper.community] = scraper.community_name

    app.add_handler(CommandHandler("status", cmd_status))
    app.add_handler(CommandHandler("restart", cmd_restart))
    app.add_handler(CommandHandler("stop", cmd_stop))
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("stats", cmd_stats))
    app.add_handler(CommandHandler("help", cmd_help))
    app.post_init = _on_startup
    app.post_shutdown = _on_shutdown
    logger.info("관리 명령어 등록 완료 (ADMIN_ID=%d)", ADMIN_ID)
