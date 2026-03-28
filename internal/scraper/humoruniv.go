package scraper

import (
	"fmt"
	"net/http"
	"strconv"
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

	// Primary: table.list_table tr with a[href*='read.html']
	doc.Find("table.list_table tr").Each(func(_ int, row *goquery.Selection) {
		aTag := row.Find("a[href*='read.html']").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		// Votes: td[width='35'] span.o
		votes := 0
		recSpan := row.Find("td[width='35'] span.o").First()
		if recSpan.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(recSpan.Text())); err == nil {
				votes = v
			}
		}

		var url string
		if strings.HasPrefix(href, "http") {
			url = href
		} else if strings.HasPrefix(href, "/") {
			url = s.baseURL + href
		} else {
			url = s.baseURL + "/board/humor/" + href
		}
		posts = append(posts, s.makePost(title, url, votes, 0, 0))
	})

	// Fallback: broader selector if primary yields nothing
	if len(posts) == 0 {
		seen := make(map[string]bool)
		doc.Find("a[href]").Each(func(_ int, aTag *goquery.Selection) {
			href, _ := aTag.Attr("href")
			if !strings.Contains(href, "read.html") {
				return
			}
			title := strings.TrimSpace(aTag.Text())
			if title == "" || len(title) < 2 {
				return
			}

			var url string
			if strings.HasPrefix(href, "http") {
				url = href
			} else if strings.HasPrefix(href, "/") {
				url = s.baseURL + href
			} else {
				url = s.baseURL + "/board/humor/" + href
			}
			if !seen[url] {
				seen[url] = true
				posts = append(posts, s.makePost(title, url, 0, 0, 0))
			}
		})
	}

	return s.filterPosts(posts), nil
}
