package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"github.com/bc1qwerty/best-archive-bot/internal/db"
	"github.com/bc1qwerty/best-archive-bot/internal/notifyhub"
	"github.com/bc1qwerty/best-archive-bot/internal/persist"
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
	//
	// ⚠ 이 상한을 넘긴 분량은 프레임워크가 **발송 없이 seen 으로 찍어 버린다** —
	//   재시도 경로가 없어 영구 유실이다(runner.go "backlog cap"). 그래서 값이
	//   「한 폴이 실제로 가져올 수 있는 최대」보다 낮으면 평상시에도 조용히 글이
	//   사라진다. 10 이던 2026-09-12~14 실측: 16회 실행 중 5회가 상한에 걸려
	//   17건이 그렇게 없어졌다(한 회차 최대 유실 7건).
	//
	//   한 폴의 구조적 최대는 (활성 커뮤니티 수) × maxPerCommunity 다 —
	//   InterleavingSource 가 커뮤니티당 maxPerCommunity 로 자르므로 그보다 많이
	//   나올 수 없다(지금 12 × 3 = 36). GitHub 스케줄러가 트리거를 흘려 폴 간격이
	//   2~6시간으로 벌어지는 것이 이 봇의 상시 조건이라(위 bot.yml 주석) 공백이
	//   길어지면 실제로 그 근처까지 찬다. 커뮤니티 한 곳 분량을 더 얹어 40 으로
	//   둔다 — 평상시엔 발화하지 않으면서, MaxPerSource 가 풀리는 사고
	//   (커뮤니티당 최대 20건 → 240건)에는 여전히 브레이크로 남는다.
	//
	//   ⚠발송량이 늘어도 텔레그램 한도에 눌려 유실되지는 않는다. 실측 발송
	//   속도는 ~1건/초(2026-09-13 10건 9.6초, 429 0회)라 40건도 runTimeout 5분
	//   안이고, 429 가 나도 프레임워크가 retry_after 를 따라 재시도한 뒤
	//   실패분은 seen 처리를 미뤄 다음 폴에서 다시 집는다.
	maxSendPerRun = 40
	// maxPerCommunity caps how many posts a single community contributes to
	// one run. Because each community's posts are popularity-sorted upstream,
	// this keeps its hottest few and prevents one busy community (e.g. a game
	// board mid-drama) from monopolizing a batch.
	maxPerCommunity = 3
	// hubChannel is the txid notification-hub channel slug. It is NOT the
	// Telegram chat id: the hub keys notifications by logical channel,
	// while Telegram delivery is handled separately by the framework
	// Notifier. It doubles as the framework bot name and store namespace
	// so all three stay in sync.
	hubChannel = "best-archive"
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

	// Legacy SQLite is kept only because its Init() creates data/posts.db,
	// which resolveDBPath (and the GHA cache) depend on. Its sent_posts
	// table is unused; framework Store owns dedup, and its retention runs
	// in closeStore below (the daemon-only runCleanup never fires here).
	legacyDB := db.New()
	if err := legacyDB.Init(); err != nil {
		log.Fatalf("legacy DB init: %v", err)
	}
	defer legacyDB.Close()
	if err := legacyDB.CleanupOldRecords(); err != nil {
		log.Printf("legacy cleanup warning: %v", err)
	}

	// Interleaving source mixes 12 communities round-robin so one chatty
	// site cannot monopolize the dispatch slots.
	scrapers := scraper.AllScrapers()
	sources := make([]core.Source, 0, len(scrapers))
	for _, s := range scrapers {
		sources = append(sources, source.NewAdapter(s))
	}
	interleaved := source.NewInterleavingSource(sources...)
	interleaved.MaxPerSource = maxPerCommunity

	ntf, err := notify.NewTelegram(config.BotToken)
	if err != nil {
		log.Fatalf("Telegram init: %v", err)
	}

	// GHA cache restores the legacy file as data/posts.db; honor it so
	// bot_seen survives the rename. New deploys without cache create
	// data/best-archive.db.
	dbPath := resolveDBPath(baseDir)
	st, err := store.Open(dbPath, hubChannel)
	if err != nil {
		log.Fatalf("framework store open: %v", err)
	}
	// The store opens in WAL mode; on this one-shot run the dedup writes
	// otherwise stay in the "-wal" sidecar, which the GHA cache does not
	// snapshot. Checkpoint into the main .db (and close) before exit so
	// bot_seen survives to the next run. Named (not just deferred) because
	// the total-fetch-failure path exits via os.Exit, which skips defers.
	closeStore := func() {
		// One-shot runs never reach the daemon-only runCleanup, so prune
		// old dedup rows here (same retention as the daemon default).
		if err := st.Cleanup(90 * 24 * time.Hour); err != nil {
			log.Printf("store cleanup warning: %v", err)
		}
		if err := persist.Checkpoint(st.DB()); err != nil {
			log.Printf("wal checkpoint warning: %v", err)
		}
		if err := st.Close(); err != nil {
			log.Printf("store close warning: %v", err)
		}
	}
	defer closeStore()
	_ = st.Subscribe(config.ChatID)

	// Set when a poll fetched nothing from any community; the process must
	// then exit nonzero so the GHA failure alert and the consecutive-failure
	// guard engage instead of the run ending green.
	fetchFailedTotal := false

	runner := bot.New(bot.Config{
		Name:            hubChannel,
		Source:          interleaved,
		Formatter:       &ArchiveFormatter{},
		Notifier:        ntf,
		Store:           st,
		ArchiveDir:      archiveDir(baseDir),
		HeartbeatDir:    heartbeatDir(),
		MaxItemsPerPoll:   maxSendPerRun,
		ArchiveRetainDays: 30,
		BootstrapMode:   os.Getenv("BOOTSTRAP_DEDUP") == "1",
		OnNewItem: func(ctx context.Context, item core.Item) error {
			return notifyhub.Push(notifyhub.Payload{
				ChannelID: hubChannel,
				Title:     item.Title,
				URL:       item.URL,
				Category:  item.Category,
			})
		},
		OnError: func(err error) {
			// A run where some communities failed but others delivered is
			// warn-level: it must stay visible in the central log without
			// showing up in dash.txid.uk's Recent Errors, which only lists
			// error/fatal and should mean "something needs attention".
			level := "error"
			var partial *source.PartialFetchError
			if errors.As(err, &partial) {
				level = "warn"
			}
			var total *source.TotalFetchError
			if errors.As(err, &total) {
				fetchFailedTotal = true
			}
			_ = notifyhub.LogPush("best-archive-bot", level, err.Error(), "")
		},
		OnPollComplete: func(ctx context.Context, n int) error {
			return notifyhub.LogPush("best-archive-bot", "info",
				fmt.Sprintf("poll complete (%d items)", n), "")
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	runner.PollOnce(ctx)

	if fetchFailedTotal {
		log.Println("=== Best Archive Bot run FAILED: 전 커뮤니티 fetch 실패 ===")
		closeStore()
		os.Exit(1)
	}

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
// resolveDBPath honors DB_PATH env first, then the legacy posts.db
// filename used by the GHA workflow cache, then falls back to the new
// hyphenated convention.
func resolveDBPath(baseDir string) string {
	if v := os.Getenv("DB_PATH"); v != "" {
		return v
	}
	legacy := filepath.Join(baseDir, "data", "posts.db")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return filepath.Join(baseDir, "data", "best-archive.db")
}

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
