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

// normalizeTitle unescapes HTML entities then collapses every run of
// whitespace -- newlines, tabs, indentation, and NBSP from layout markup --
// into a single space. Scrapers read titles via goquery Selection.Text(),
// which concatenates nested text nodes verbatim; sites whose title anchor
// wraps nested markup (e.g. ruliweb's "flex" anchor, humoruniv) would
// otherwise keep embedded newlines and indentation inside the title.
func normalizeTitle(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(s)), " ")
}

// makePost creates a Post with a whitespace-normalized, html-unescaped title.
func (b *baseScraper) makePost(title, url string, votes, views, comments int) Post {
	return Post{
		Title:         normalizeTitle(title),
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

// AllScrapers returns a slice of the active community scrapers.
//
// dvdprime is intentionally omitted: dvdprime.com rate-limits GitHub Actions'
// datacenter IP range and returns HTTP 429 on nearly every run (verified
// 2026-05-20 — 13/15 runs failed, while the same request from a Korean
// residential IP returns 200). Retrying inside the run reuses the same blocked
// IP, so it does not help. NewDvdprimeScraper and dvdprime.go are kept so the
// source can be restored in one line if the bot ever moves to a Korean-IP host.
func AllScrapers() []Scraper {
	return []Scraper{
		NewBobaedreamScraper(),
		NewClienScraper(),
		NewCook82Scraper(),
		NewDcinsideScraper(),
		NewEtolandScraper(),
		NewHumorunivScraper(),
		NewInvenScraper(),
		NewMlbparkScraper(),
		NewNatepannScraper(),
		NewPpomppuScraper(),
		NewRuliwebScraper(),
		NewTheqooScraper(),
	}
}
