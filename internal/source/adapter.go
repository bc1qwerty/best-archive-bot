package source

import (
	"context"
	"net/http"
	"net/http/cookiejar"
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
	// 쿠키 저장소가 필요한 이유: 뽐뿌는 데이터센터 IP 에 302 → `?ppck=1` 로
	// `pp_ck` 쿠키를 심고 그 쿠키 없이 돌아오면 403 을 준다(2026-10-01~ 9일간
	// 전 실행 403). cookiejar.New(nil) 은 에러를 내지 않는다.
	jar, _ := cookiejar.New(nil)
	return &ScraperAdapter{
		scraper: s,
		client:  &http.Client{Timeout: 30 * time.Second, Jar: jar},
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
