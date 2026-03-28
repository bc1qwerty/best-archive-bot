package scraper

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var ruliwebReplyRe = regexp.MustCompile(`\d+`)

// RuliwebScraper scrapes 루리웹 best selection.
type RuliwebScraper struct {
	baseScraper
}

func NewRuliwebScraper() *RuliwebScraper {
	return &RuliwebScraper{
		baseScraper: baseScraper{
			community:     "ruliweb",
			communityName: "루리웹",
			baseURL:       "https://bbs.ruliweb.com",
			encoding:      "utf-8",
		},
	}
}

func (s *RuliwebScraper) Name() string { return s.communityName }

func (s *RuliwebScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/best/selection", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	doc.Find("tr.table_body").Each(func(_ int, row *goquery.Selection) {
		aTag := row.Find("a.deco").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" {
			return
		}

		votes := 0
		recTd := row.Find("td.recomd").First()
		if recTd.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(recTd.Text())); err == nil {
				votes = v
			}
		}

		comments := 0
		replySpan := row.Find("span.num_reply").First()
		if replySpan.Length() > 0 {
			m := ruliwebReplyRe.FindString(replySpan.Text())
			if m != "" {
				if v, err := strconv.Atoi(m); err == nil {
					comments = v
				}
			}
		}

		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		posts = append(posts, s.makePost(title, url, votes, 0, comments))
	})

	return s.filterPosts(posts), nil
}
