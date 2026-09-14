# Best Archive Bot

## Language
- Respond in Korean (한국어로 응답)

## Description
Korean community best-post aggregator. Scrapes popular posts from 12 active communities (DCInside, Theqoo, Natepann, Clien, Bobaedream, MLBPark, Ppomppu, Ruliweb, Inven, Cook82, Humoruniv, Etoland — DVDPrime is kept in code but disabled: it 429s GitHub Actions' IP range) and sends to Telegram via the txid-bot-framework. Designed as run-once job.

## Tech Stack
- **Language**: Go 1.24
- **Database**: SQLite (modernc.org/sqlite, pure Go)
- **Scraping**: goquery
- **Framework**: txid-bot-framework (`replace ../txid-bot-framework` in go.mod) — runner, dedup store, Telegram notifier
- **Config**: YAML (communities), godotenv (.env)

## Project Structure
```
main.go                    # Run-once: scrape all → interleave → framework runner dispatches
config/
  communities.yaml         # Community definitions
internal/
  config/                  # Env config loader
  db/                      # Legacy SQLite — kept only because Init() creates data/posts.db (cache path coupling)
  notifyhub/               # txid notification-hub client (3-bot copy — sync siblings on change)
  persist/                 # WAL checkpoint for the one-shot GHA cache
  scraper/                 # 13 community scrapers (12 active; dvdprime disabled)
  source/                  # framework Source adapters + interleaving
run_bot.bat                # 유물 (python 시절 잔재) — 사용 안 함
```

## Build & Run
```bash
go build -o best-archive-bot .
# Set env: BOT_TOKEN, CHAT_ID
./best-archive-bot          # runs once, exits
```

## Environment Variables
- `BOT_TOKEN` - **Required** Telegram bot token
- `CHAT_ID` - **Required** Telegram chat ID

## Deployment
- GitHub Actions: `.github/workflows/bot.yml`, cron `*/15 * * * *` (+ workflow_dispatch with bootstrap input)
- The workflow checks out `bc1qwerty/txid-bot-framework@main` as a sibling to satisfy the go.mod `replace`
- Push to main = 자동 반영 (다음 cron 틱부터)
- dedup 상태의 유일한 사본은 GHA 캐시(`data/posts.db*`) — 유실 감지·부트스트랩 로직은 bot.yml 주석 참조
- Max 10 posts per run, max 3 per community per run

## Status
Active. GitHub Actions scheduled execution (acer/dell cron 시절은 종료).
