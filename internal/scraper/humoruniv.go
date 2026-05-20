package scraper

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// HumorunivScraper scrapes 웃긴대학 best (EUC-KR).
type HumorunivScraper struct {
	baseScraper
}

func NewHumorunivScraper() *HumorunivScraper {
	return &HumorunivScraper{
		baseScraper: baseScraper{
			community:     "humoruniv",
			communityName: "웃긴대학",
			baseURL:       "http://web.humoruniv.com",
			encoding:      "euc-kr",
		},
	}
}

func (s *HumorunivScraper) Name() string { return s.communityName }

func (s *HumorunivScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/board/humor/board_best.html", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	seen := make(map[string]bool)

	// Each post title lives in <span id="title_chk_<table>-<number>">. The
	// enclosing <a> also wraps the comment-count span (.list_comment_num)
	// and a vote span, so reading the anchor text would fold
	// "[77] 답글추천 +481" into the title -- read the title span directly.
	doc.Find("span[id^='title_chk_']").Each(func(_ int, titleSpan *goquery.Selection) {
		title := strings.TrimSpace(titleSpan.Text())
		if title == "" {
			return
		}
		href, exists := titleSpan.Closest("a[href*='read.html']").Attr("href")
		if !exists || href == "" {
			return
		}

		var url string
		switch {
		case strings.HasPrefix(href, "http"):
			url = href
		case strings.HasPrefix(href, "/"):
			url = s.baseURL + href
		default:
			url = s.baseURL + "/board/humor/" + href
		}
		if seen[url] {
			return
		}
		seen[url] = true
		posts = append(posts, s.makePost(title, url, 0, 0, 0))
	})

	return s.filterPosts(posts), nil
}
