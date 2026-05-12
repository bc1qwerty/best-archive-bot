package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"github.com/bc1qwerty/best-archive-bot/internal/db"
	"github.com/bc1qwerty/best-archive-bot/internal/notifyhub"
	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
	"github.com/bc1qwerty/best-archive-bot/internal/source"
	"github.com/bc1qwerty/txid-bot-framework/pkg/bot"
	"github.com/bc1qwerty/txid-bot-framework/pkg/core"
	"github.com/bc1qwerty/txid-bot-framework/pkg/notify"
	"github.com/bc1qwerty/txid-bot-framework/pkg/store"
)

const (
	// runTimeout caps the overall run so a single slow scraper cannot
	// stall a cron tick.
	runTimeout = 5 * time.Minute
	// maxSendPerRun limits total dispatched items per cron invocation.
	maxSendPerRun = 10
)

// ArchiveFormatter renders a community best-post as Telegram HTML.
type ArchiveFormatter struct{}

func (f *ArchiveFormatter) Format(item core.Item) core.Message {
	text := fmt.Sprintf("🔥 <b>[%s]</b> 인기글\n\n%s\n\n🔗 <a href=\"%s\">본문 보기</a>",
		item.Category, item.Title, item.URL)
	return core.Message{
		Text:      text,
		ParseMode: "HTML",
	}
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Println("=== Best Archive Bot (Framework Mode) starting ===")
	_ = notifyhub.LogPush("best-archive-bot", "info", "run started", "")

	baseDir := resolveBaseDir()
	config.Load(baseDir)

	if config.BotToken == "" || config.ChatID == "" {
		log.Fatal("BOT_TOKEN / CHAT_ID 환경변수가 필요합니다")
	}

	// Legacy SQLite owns the retention/cleanup policy (RecordExpireHours).
	// Framework Store handles only framework-level dedup; we keep both
	// during the migration window.
	legacyDB := db.New()
	if err := legacyDB.Init(); err != nil {
		log.Fatalf("legacy DB init: %v", err)
	}
	defer legacyDB.Close()
	if err := legacyDB.CleanupOldRecords(); err != nil {
		log.Printf("legacy cleanup warning: %v", err)
	}

	// Interleaving source mixes 13 communities round-robin so one chatty
	// site cannot monopolize the dispatch slots.
	scrapers := scraper.AllScrapers()
	sources := make([]core.Source, 0, len(scrapers))
	for _, s := range scrapers {
		sources = append(sources, source.NewAdapter(s))
	}
	interleaved := source.NewInterleavingSource(sources...)

	ntf, err := notify.NewTelegram(config.BotToken)
	if err != nil {
		log.Fatalf("Telegram init: %v", err)
	}

	dbPath := filepath.Join(baseDir, "data", "best-archive.db")
	st, err := store.Open(dbPath, "best-archive")
	if err != nil {
		log.Fatalf("framework store open: %v", err)
	}
	_ = st.Subscribe(config.ChatID)

	runner := bot.New(bot.Config{
		Name:            "best-archive",
		Source:          interleaved,
		Formatter:       &ArchiveFormatter{},
		Notifier:        ntf,
		Store:           st,
		ArchiveDir:      archiveDir(baseDir),
		HeartbeatDir:    heartbeatDir(),
		MaxItemsPerPoll: maxSendPerRun,
		OnNewItem: func(ctx context.Context, item core.Item) error {
			return notifyhub.Push(notifyhub.Payload{
				ChannelID: config.ChatID,
				Title:     item.Title,
				URL:       item.URL,
				Category:  item.Category,
			})
		},
		OnError: func(err error) {
			_ = notifyhub.LogPush("best-archive-bot", "error", err.Error(), "")
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	runner.PollOnce(ctx)

	_ = notifyhub.LogPush("best-archive-bot", "info", "run finished", "")
	log.Println("=== Best Archive Bot run complete ===")
}

func resolveBaseDir() string {
	if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, "config", "communities.yaml")); err == nil {
			return wd
		}
	}
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	return filepath.Dir(exe)
}

// archiveDir resolves the raw-JSONL backup directory. ARCHIVE_DIR env
// overrides; otherwise <baseDir>/data/archive so behavior is uniform
// across hosts (office, dell, acer, VPS) without hardcoded paths.
func archiveDir(baseDir string) string {
	if v := os.Getenv("ARCHIVE_DIR"); v != "" {
		return v
	}
	return filepath.Join(baseDir, "data", "archive")
}

// heartbeatDir resolves the liveness file directory. HEARTBEAT_DIR env
// overrides; otherwise ~/.txid-bots/heartbeats so dash.txid.uk can keep
// reading the same convention regardless of the host's username.
func heartbeatDir() string {
	if v := os.Getenv("HEARTBEAT_DIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".txid-bots", "heartbeats")
}
