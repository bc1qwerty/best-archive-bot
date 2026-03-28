package scraper

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// DcinsideScraper scrapes DC인사이드 best gallery.
type DcinsideScraper struct {
	baseScraper
}

func NewDcinsideScraper() *DcinsideScraper {
	return &DcinsideScraper{
		baseScraper: baseScraper{
			community:     "dcinside",
			communityName: "DC인사이드",
			baseURL:       "https://gall.dcinside.com",
			encoding:      "utf-8",
		},
	}
}

func (s *DcinsideScraper) Name() string { return s.communityName }

func (s *DcinsideScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/board/lists/?id=dcbest", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	doc.Find("tr.ub-content").Each(func(_ int, row *goquery.Selection) {
		// Title link: td.gall_tit a (not .reply_numbox)
		aTag := row.Find("td.gall_tit a").Not(".reply_numbox").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		votes := 0
		recTd := row.Find("td.gall_recommend")
		if recTd.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(recTd.Text())); err == nil {
				votes = v
			}
		}

		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		posts = append(posts, s.makePost(title, url, votes, 0, 0))
	})

	return s.filterPosts(posts), nil
}
