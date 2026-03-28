package scraper

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// TheqooScraper scrapes 더쿠 hot posts.
type TheqooScraper struct {
	baseScraper
}

func NewTheqooScraper() *TheqooScraper {
	return &TheqooScraper{
		baseScraper: baseScraper{
			community:     "theqoo",
			communityName: "더쿠",
			baseURL:       "https://theqoo.net",
			encoding:      "utf-8",
		},
	}
}

func (s *TheqooScraper) Name() string { return s.communityName }

func (s *TheqooScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/hot", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	doc.Find("table.theqoo_board_table tbody tr").Each(func(_ int, row *goquery.Selection) {
		tds := row.Find("td")
		if tds.Length() < 3 {
			return
		}

		// Skip notices
		first := strings.TrimSpace(tds.First().Text())
		if strings.Contains(first, "공지") || strings.Contains(first, "이벤트") {
			return
		}

		aTag := row.Find("td.title a").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		// Views: last td
		views := 0
		lastTd := tds.Last()
		viewsText := strings.TrimSpace(strings.ReplaceAll(lastTd.Text(), ",", ""))
		if v, err := strconv.Atoi(viewsText); err == nil {
			views = v
		}

		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		posts = append(posts, s.makePost(title, url, 0, views, 0))
	})

	return s.filterPosts(posts), nil
}
