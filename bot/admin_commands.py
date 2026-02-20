import logging
from functools import wraps

from telegram import BotCommand, Update
from telegram.ext import Application, CommandHandler, ContextTypes

from config.settings import ADMIN_ID

logger = logging.getLogger(__name__)


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


def _stop_scraping(context: ContextTypes.DEFAULT_TYPE) -> int:
    """스크래핑 job 모두 제거, 제거 개수 반환"""
    jobs = context.application.job_queue.jobs()
    count = 0
    for job in jobs:
        if job.name.startswith("scrape_"):
            job.schedule_removal()
            count += 1
    return count


def _start_scraping(context: ContextTypes.DEFAULT_TYPE) -> int:
    """스크래핑 job 재등록, 등록 개수 반환"""
    from bot.telegram_bot import scrape_one_community, SCRAPER_SCHEDULE
    job_queue = context.application.job_queue

    count = 0
    for scraper, interval_min, first_sec in SCRAPER_SCHEDULE:
        job_queue.run_repeating(
            scrape_one_community,
            interval=interval_min * 60,
            first=first_sec,
            data=scraper,
            name=f"scrape_{scraper.community}",
        )
        count += 1
    return count


@admin_only
async def cmd_status(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 상태 조회"""
    jobs = context.application.job_queue.jobs()
    scrape_jobs = [j for j in jobs if j.name.startswith("scrape_")]
    if scrape_jobs:
        names = ", ".join(j.name.replace("scrape_", "") for j in scrape_jobs)
        await update.message.reply_text(
            f"✅ 스크래핑 작동 중 ({len(scrape_jobs)}개)\n{names}"
        )
    else:
        await update.message.reply_text("⏹ 스크래핑 중지 상태")


@admin_only
async def cmd_stop(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 중지"""
    if not _is_scraping_active(context):
        await update.message.reply_text("⏹ 이미 중지 상태입니다.")
        return
    count = _stop_scraping(context)
    await update.message.reply_text(f"⏹ 스크래핑 중지 완료 ({count}개 job 제거)")
    logger.info("관리자 명령: 스크래핑 중지 (%d개)", count)


@admin_only
async def cmd_start(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 시작"""
    if _is_scraping_active(context):
        await update.message.reply_text("✅ 이미 작동 중입니다.")
        return
    count = _start_scraping(context)
    await update.message.reply_text(f"▶ 스크래핑 시작 ({count}개 job 등록)")
    logger.info("관리자 명령: 스크래핑 시작 (%d개)", count)


@admin_only
async def cmd_restart(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """스크래핑 재시작"""
    _stop_scraping(context)
    count = _start_scraping(context)
    await update.message.reply_text(f"🔄 스크래핑 재시작 완료 ({count}개 job 등록)")
    logger.info("관리자 명령: 스크래핑 재시작 (%d개)", count)


@admin_only
async def cmd_help(update: Update, context: ContextTypes.DEFAULT_TYPE):
    """명령어 목록"""
    text = (
        "🤖 <b>BestArchiveBot 관리 명령어</b>\n\n"
        "/status — 스크래핑 상태 조회\n"
        "/start — 스크래핑 시작\n"
        "/stop — 스크래핑 중지\n"
        "/restart — 스크래핑 재시작\n"
        "/help — 명령어 목록"
    )
    await update.message.reply_text(text, parse_mode="HTML")


async def _set_bot_menu(app: Application) -> None:
    """봇 시작 시 커맨드 메뉴 등록"""
    commands = [
        BotCommand("status", "스크래핑 상태 조회"),
        BotCommand("start", "스크래핑 시작"),
        BotCommand("stop", "스크래핑 중지"),
        BotCommand("restart", "스크래핑 재시작"),
        BotCommand("help", "명령어 목록"),
    ]
    await app.bot.set_my_commands(commands)
    logger.info("봇 커맨드 메뉴 등록 완료")


def register_admin_commands(app: Application) -> None:
    """관리 명령어 핸들러 등록"""
    app.add_handler(CommandHandler("status", cmd_status))
    app.add_handler(CommandHandler("restart", cmd_restart))
    app.add_handler(CommandHandler("stop", cmd_stop))
    app.add_handler(CommandHandler("start", cmd_start))
    app.add_handler(CommandHandler("help", cmd_help))
    app.post_init = _set_bot_menu
    logger.info("관리 명령어 등록 완료 (ADMIN_ID=%d)", ADMIN_ID)
