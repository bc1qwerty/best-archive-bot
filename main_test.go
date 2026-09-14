package main

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
	"github.com/bc1qwerty/best-archive-bot/internal/source"
	"github.com/bc1qwerty/txid-bot-framework/pkg/bot"
	"github.com/bc1qwerty/txid-bot-framework/pkg/core"
	"github.com/bc1qwerty/txid-bot-framework/pkg/store"
)

// fixedSource 는 커뮤니티 한 곳이 준비된 글을 그대로 돌려주는 가짜 소스다.
type fixedSource struct {
	name  string
	items []core.Item
}

func (s *fixedSource) Name() string { return s.name }

func (s *fixedSource) Fetch(context.Context) ([]core.Item, error) { return s.items, nil }

// recordingNotifier 는 실제로 발송된 아이템만 기록한다(네트워크 없음).
type recordingNotifier struct {
	sent []string
}

func (n *recordingNotifier) Name() string { return "recording" }

func (n *recordingNotifier) Send(_ context.Context, _ string, msg core.Message) error {
	n.sent = append(n.sent, msg.Text)
	return nil
}

// TestMaxSendPerRunCoversFullFetch 는 «폴 간격이 길어져 한 회차에 유입이 몰려도
// 초과분을 발송 없이 seen 으로 버리지 않는다» 를 고정한다.
//
// GitHub 스케줄러가 트리거를 흘려 폴 간격이 2~6시간으로 벌어지는 것이 이 봇의
// 상시 조건이고, maxSendPerRun 이 10 이던 2026-09-12~14 실측에서 16회 중 5회가
// 상한에 걸려 17건이 영구 유실됐다(프레임워크의 backlog cap 은 초과분을 발송
// 없이 seen 처리하고 다시 집지 않는다). 한 폴이 구조적으로 가져올 수 있는
// 최대치를 통째로 밀어 넣고 한 건도 사라지지 않는지 본다.
func TestMaxSendPerRunCoversFullFetch(t *testing.T) {
	communities := len(scraper.AllScrapers())
	// InterleavingSource 가 커뮤니티당 maxPerCommunity 로 자르므로 한 폴의
	// 아이템 수는 이 값을 넘을 수 없다.
	burst := communities * maxPerCommunity

	sources := make([]core.Source, 0, communities)
	for c := 0; c < communities; c++ {
		items := make([]core.Item, 0, maxPerCommunity)
		for p := 0; p < maxPerCommunity; p++ {
			// 제목까지 서로 달라야 한다 — InterleavingSource 는 같은 제목을
			// 크로스포스트로 보고 하나로 접는다.
			url := fmt.Sprintf("https://example.test/c%d/p%d", c, p)
			items = append(items, core.Item{
				ID:       url,
				Title:    fmt.Sprintf("커뮤니티%d 인기글%d", c, p),
				URL:      url,
				Category: fmt.Sprintf("커뮤니티%d", c),
			})
		}
		sources = append(sources, &fixedSource{name: fmt.Sprintf("community%d", c), items: items})
	}

	interleaved := source.NewInterleavingSource(sources...)
	interleaved.MaxPerSource = maxPerCommunity

	st, err := store.Open(filepath.Join(t.TempDir(), "best-archive-test.db"), hubChannel)
	if err != nil {
		t.Fatalf("store open: %v", err)
	}
	defer st.Close()
	if err := st.Subscribe("test-chat"); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	ntf := &recordingNotifier{}
	runner := bot.New(bot.Config{
		Name:            hubChannel,
		Source:          interleaved,
		Formatter:       &ArchiveFormatter{},
		Notifier:        ntf,
		Store:           st,
		MaxItemsPerPoll: maxSendPerRun,
	})
	runner.PollOnce(context.Background())

	if len(ntf.sent) != burst {
		t.Fatalf("한 폴 최대 유입 %d건(커뮤니티 %d × %d) 중 %d건만 발송됐다 — "+
			"나머지 %d건은 발송 없이 seen 처리돼 영구 유실된다 (maxSendPerRun=%d)",
			burst, communities, maxPerCommunity, len(ntf.sent), burst-len(ntf.sent), maxSendPerRun)
	}

	// 발송된 것이 곧 전부라는 것을 dedup 상태로도 확인한다: 다시 폴하면
	// 새 아이템이 하나도 없어야 하고(전부 seen), 한 건도 더 나가지 않아야 한다.
	runner.PollOnce(context.Background())
	if len(ntf.sent) != burst {
		t.Fatalf("두 번째 폴에서 %d건이 더 발송됐다 — dedup 이 깨졌다", len(ntf.sent)-burst)
	}
}
