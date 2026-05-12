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
	posts, err := a.scraper.FetchBestPosts(a.client)
	if err != nil {
		return nil, err
	}

	var items []core.Item
	for _, p := range posts {
		items = append(items, core.Item{
			ID:       p.URL,
			Title:    p.Title,
			URL:      p.URL,
			Category: p.CommunityName,
		})
	}
	return items, nil
}
