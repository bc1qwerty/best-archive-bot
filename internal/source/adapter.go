package source

import (
	"context"
	"net/http"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
	"github.com/bc1qwerty/txid-bot-framework/pkg/core"
)

// ScraperAdapter converts a community scraper to a framework Source.
type ScraperAdapter struct {
	scraper scraper.Scraper
	client  *http.Client
}

func NewAdapter(s scraper.Scraper) *ScraperAdapter {
	return &ScraperAdapter{
		scraper: s,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *ScraperAdapter) Name() string {
	return a.scraper.Name()
}

func (a *ScraperAdapter) Fetch(ctx context.Context) ([]core.Item, error) {
	// ctx 는 스크레이퍼 HTTP 계층(FetchBestPosts/fetchHTML)에 전파되지 않고,
	// 개별 요청은 client 의 30s Timeout 으로만 묶인다. 그래서 5분 run 예산이
	// 이미 소진됐으면 여기서 멈춰 DeadlineExceeded 를 그대로 올린다 — 진행 중인
	// 스크레이퍼는 못 끊지만, 남은 커뮤니티를 계속 긁으며 예산 초과를 무음으로
	// 지나치는 것은 막고, framework 의 «예산 초과» 경보 경로가 살아난다.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	posts, err := a.scraper.FetchBestPosts(a.client)
	if err != nil {
		return nil, err
	}

	// Hottest-first within this community so the interleaving source and the
	// per-poll cap select each community's best posts before its weaker ones.
	sortByPopularity(posts)

	var items []core.Item
	for _, p := range posts {
		// Canonicalize so the same post reached via different list pages or
		// tracking params dedups to a single bot_seen entry.
		u := NormalizeURL(p.URL)
		items = append(items, core.Item{
			ID:       u,
			Title:    p.Title,
			URL:      u,
			Category: p.CommunityName,
		})
	}
	return items, nil
}
