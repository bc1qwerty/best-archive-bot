package scraper

import (
	"html"
	"net/http"
	"strings"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
)

// Post represents a single community post.
type Post struct {
	Title         string
	URL           string
	Community     string // English key (e.g. "dcinside")
	CommunityName string // Korean name (e.g. "DC인사이드")
	Votes         int
	Views         int
	Comments      int
}

// Scraper defines the interface all community scrapers must implement.
type Scraper interface {
	Name() string
	FetchBestPosts(client *http.Client) ([]Post, error)
}

// baseScraper provides common fields and helpers shared by all scrapers.
type baseScraper struct {
	community     string
	communityName string
	baseURL       string
	encoding      string
	dataRequired  bool // if true, posts with 0/0/0 stats are filtered out
}

// makePost creates a Post with html-unescaped title.
func (b *baseScraper) makePost(title, url string, votes, views, comments int) Post {
	return Post{
		Title:         html.UnescapeString(strings.TrimSpace(title)),
		URL:           url,
		Community:     b.community,
		CommunityName: b.communityName,
		Votes:         votes,
		Views:         views,
		Comments:      comments,
	}
}

// shouldInclude checks popularity thresholds. ANY one metric passing is enough.
func (b *baseScraper) shouldInclude(p Post) bool {
	hasVotes := p.Votes > 0
	hasViews := p.Views > 0
	hasComments := p.Comments > 0

	if !hasVotes && !hasViews && !hasComments {
		if b.dataRequired {
			return false // individual page crawl failed
		}
		return true // data not provided → exempt
	}
	if hasVotes && p.Votes >= config.MinVotes {
		return true
	}
	if hasViews && p.Views >= config.MinViews {
		return true
	}
	if hasComments && p.Comments >= config.MinComments {
		return true
	}
	return false
}

// filterPosts applies URL validation, popularity filter, and max-posts limit.
func (b *baseScraper) filterPosts(posts []Post) []Post {
	var result []Post
	for _, p := range posts {
		if !strings.HasPrefix(p.URL, "http") {
			continue
		}
		if !b.shouldInclude(p) {
			continue
		}
		result = append(result, p)
		if len(result) >= config.MaxPostsPerCommunity {
			break
		}
	}
	return result
}
