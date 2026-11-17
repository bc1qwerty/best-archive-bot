package main

import (
	"context"
	"errors"
	"fmt"
	"html"
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
	//   ⚠한 실행에 실제로 나가는 것은 20건 안팎이다. 21번째 근처부터 텔레그램이
	//   429(Too Many Requests: retry after N)를 내고 허브도 같은 시점에 429
	//   (Too many verify requests)를 낸다 — 2026-11-17 실측: 후보 32건인 실행
	//   두 번 모두 20건 발송, 12건 보류. 10건일 때(2026-09-13, 9.6초)는 429 가
	//   없었다. 보류분은 seen 처리가 미뤄져 다음 폴에서 다시 나가므로 바로
	//   유실되지는 않지만, 같은 글이 5번 연속 보류되면 프레임워크가 포기한다.
	//   후보가 계속 20건을 넘으면 그 길로 유실이 생긴다.
	maxSendPerRun = 40
	// maxPerCommunity caps how many posts a single community contributes to
	// one run. Because each community's posts are popularity-sorted upstream,
	// this keeps its hottest few and prevents one busy community (e.g. a game
	// board mid-drama) from monopolizing a batch.
	maxPerCommunity = 3
	// titleDedupWindow: 같은 제목을 다른 커뮤니티가 보낸 지 이 시간 안이면 건너뛴다.
	// 2026-11-17 실측에서 커뮤니티 간 재등장은 53시간 안에 몰려 있었다.
	titleDedupWindow = 72 * time.Hour
	// hubChannel is the txid notification-hub channel slug. It is NOT the
	// Telegram chat id: the hub keys notifications by logical channel,
	// while Telegram delivery is handled separately by the framework
	// Notifier. It doubles as the framework bot name and store namespace
	// so all three stay in sync.
	hubChannel = "best-archive"
)

// sameTitleDistinct: 제목이 같아도 별개 글인 커뮤니티. 여기서는 같은 커뮤니티 안의
// 같은 제목을 거르지 않는다(다른 커뮤니티와의 중복은 그대로 거른다).
// 2026-11-17 실측(30일): 이토랜드는 «아이브 장원영» 32건처럼 인물 이름만 제목인
// 별개 글이 70종 넘게 반복됐고, 나머지 커뮤니티의 같은 제목은 재등록이었다.
var sameTitleDistinct = map[string]bool{"이토랜드": true}

// ArchiveFormatter renders a community best-post as Telegram HTML.
type ArchiveFormatter struct{}

func (f *ArchiveFormatter) Format(item core.Item) core.Message {
	// ⚠ParseMode HTML 메시지에 값을 그대로 끼우면 텔레그램이 파싱에서 거절해
	// **그 글은 영영 도착하지 않는다**. 제목에 «<» 가 든 인기글이 실제로
	// 그렇게 사라졌다(실로그 send error: can't parse entities). 커뮤니티
	// 제목은 <-_->, <속보>, 부등호 비교처럼 «<» 를 예사로 쓴다.
	text := fmt.Sprintf("🔥 <b>[%s]</b> 인기글\n\n%s\n\n🔗 <a href=\"%s\">본문 보기</a>",
		html.EscapeString(item.Category),
		html.EscapeString(item.Title),
		html.EscapeString(item.URL))
	return core.Message{
		Text:      text,
		ParseMode: "HTML",
	}
}

// frameworkRev 는 빌드 시 -ldflags 로 주입된다(CI 의 "Capture framework revision").
// go.mod 의 replace 때문에 빌드정보에는 프레임워크가 (devel) 로만 남아, 이것이 없으면
// 프로드 바이너리가 어느 프레임워크 커밋으로 만들어졌는지 알 방법이 없다.
// 로컬 빌드에서는 "unknown" 이고, 그 자체가 «릴리스본이 아니다» 라는 신호다.
var frameworkRev = "unknown"

func main() {
	log.SetFlags(log.Ldate | log.Ltime)
	log.Printf("=== Best Archive Bot (Framework Mode) starting === framework=%s", frameworkRev)
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

	// 커뮤니티당 3건은 «아직 안 보낸 글» 중에서 고른다(interleave.go IsSeen 주석).
	// 키는 러너가 쓰는 것과 같아야 한다 — 하위 소스 스탬프가 없으므로 소스 이름이다.
	interleaved.IsSeen = func(id string) (bool, error) { return st.IsSeen(interleaved.Name(), id) }
	// 최근에 나간 제목은 건너뛴다(persist.Titles 주석). 걸린 글은
	// seen 으로 찍는다 — 안 찍으면 기간이 지난 뒤 며칠 묵은 글로 뒤늦게 나간다.
	titles, err := persist.OpenTitles(st.DB())
	if err != nil {
		log.Fatalf("title store open: %v", err)
	}
	if err := titles.Cleanup(titleDedupWindow); err != nil {
		log.Printf("title cleanup warning: %v", err)
	}
	interleaved.DupTitle = func(key string, it core.Item) bool {
		dup, err := titles.Duplicate(key, it.Category, it.ID, !sameTitleDistinct[it.Category], titleDedupWindow)
		if err != nil || !dup {
			return false
		}
		if err := st.MarkSeen(interleaved.Name(), it.ID); err != nil {
			log.Printf("dup-title mark seen warning (item=%s): %v", it.ID, err)
		}
		log.Printf("[%s] 이미 나간 제목이라 건너뜀: %s", it.Category, it.Title)
		return true
	}
	// 부트스트랩은 지금 목록을 통째로 seen 으로 찍어야 한다. 3건만 찍으면 나머지
	// 통과 글 전부가 다음 실행부터 새 글로 쏟아진다.
	if os.Getenv("BOOTSTRAP_DEDUP") == "1" {
		interleaved.MaxPerSource = 0
	}

	// Set when a poll fetched nothing from any community; the process must
	// then exit nonzero so the GHA failure alert and the consecutive-failure
	// guard engage instead of the run ending green.
	fetchFailedTotal := false

	runner := bot.New(bot.Config{
		Name:              hubChannel,
		Source:            interleaved,
		Formatter:         &ArchiveFormatter{},
		Notifier:          ntf,
		Store:             st,
		ArchiveDir:        archiveDir(baseDir),
		HeartbeatDir:      heartbeatDir(),
		MaxItemsPerPoll:   maxSendPerRun,
		ArchiveRetainDays: 30,
		BootstrapMode:     os.Getenv("BOOTSTRAP_DEDUP") == "1",
		OnNewItem: func(ctx context.Context, item core.Item) error {
			if key := source.TitleKey(item.Title); key != "" {
				if err := titles.Record(key, item.Category, item.ID); err != nil {
					log.Printf("title record warning (item=%s): %v", item.ID, err)
				}
			}
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
			// ⚠**구조적 실패는 warn 으로 내리지 않는다(2026-10-01).** IP 차단은 다음
			// 실행에 저절로 낫지만 «요청 200 + 목록 0행» 은 사람이 셀렉터를 고쳐야
			// 낫는다. 그 둘을 한 등급에 묶어 뒀더니 이토랜드가 사이트를 다시 지은 뒤
			// 0행 경보가 **16일간 하루 ~100건씩 쌓이는데 대시보드에도 안 뜨고 텔레그램도
			// 안 갔다** — 그 커뮤니티 수집이 통째로 멈춘 것을 아무도 몰랐다.
			// errors.Join 은 다중 Unwrap 이라 errors.As 가 합쳐진 사유까지 훑는다.
			var layout *scraper.LayoutError
			if errors.As(err, &layout) {
				level = "error"
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
