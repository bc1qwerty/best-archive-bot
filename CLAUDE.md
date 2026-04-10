# Best Archive Bot

## Language
- Respond in Korean (한국어로 응답)

## Description
Korean community best-post aggregator. Scrapes popular posts from 13 communities (DCInside, Theqoo, Natepann, Clien, Bobaedream, MLBPark, Ppomppu, Ruliweb, Inven, Cook82, Humoruniv, Etoland, DVDPrime) and sends to Telegram. Designed as run-once cron job.

## Tech Stack
- **Language**: Go 1.24
- **Database**: SQLite (modernc.org/sqlite, pure Go)
- **Scraping**: goquery
- **Notifications**: Telegram Bot API (raw HTTP)
- **Config**: YAML (communities), godotenv (.env)

## Project Structure
```
main.go                    # Run-once: scrape all → filter unsent → interleave → send via Telegram
config/
  communities.yaml         # Community definitions
internal/
  bot/                     # Message formatting
  config/                  # Env config loader
  db/                      # SQLite dedup DB
  scraper/                 # 13 community scrapers (dcinside, theqoo, clien, etc.)
  telegram/                # Telegram sender
run_bot.bat                # Windows launcher
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
- Designed for cron execution (run-once, not a daemon)
- Max 10 posts per run, 20 backlog cap, 3s delay between sends
- `run_bot.bat` for Windows scheduled tasks

## Status
Active. Cron-based execution on acer or dell.
