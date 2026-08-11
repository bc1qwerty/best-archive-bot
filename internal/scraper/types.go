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

// noticePrefixes are bracketed admin tags that mark board notices rather
// than user posts. Real community/gallery tags (e.g. [싱갤], [유머]) never
// use these words, so matching them is safe.
var noticePrefixes = []string{
	"[공지]", "【공지】", "[안내]", "[필독]", "[이벤트]", "[운영]", "[알림]",
}

// isNoticeTitle reports whether a title looks like a board notice/announcement
// rather than a real post. It is intentionally conservative — bracketed admin
// tags plus a couple of unambiguous phrases — so it does not drop genuine
// popular posts. Per-scraper DOM markers (dcinside num, clien/theqoo classes)
// remain the primary defense; this is a cross-community safety net.
func isNoticeTitle(title string) bool {
	compact := strings.ReplaceAll(title, " ", "")
	for _, p := range noticePrefixes {
		if strings.HasPrefix(compact, p) {
			return true
		}
	}
	if strings.Contains(compact, "갤러리이용안내") || strings.Contains(compact, "운영원칙") {
		return true
	}
	return false
}

// isAllDigits reports whether s is non-empty and consists only of ASCII
// digits. Used to keep only real numbered posts on list pages whose notice
// / survey / ad rows carry a non-numeric marker in the number column.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// filterPosts applies URL validation, notice/popularity filters, and the
// max-posts limit.
func (b *baseScraper) filterPosts(posts []Post) []Post {
	var result []Post
	for _, p := range posts {
		if !strings.HasPrefix(p.URL, "http") {
			continue
		}
		if isNoticeTitle(p.Title) {
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
