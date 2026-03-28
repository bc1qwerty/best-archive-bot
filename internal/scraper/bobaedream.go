package scraper

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// BobaedreamScraper scrapes 보배드림 best posts.
type BobaedreamScraper struct {
	baseScraper
}

func NewBobaedreamScraper() *BobaedreamScraper {
	return &BobaedreamScraper{
		baseScraper: baseScraper{
			community:     "bobaedream",
			communityName: "보배드림",
			baseURL:       "https://www.bobaedream.co.kr",
			encoding:      "utf-8",
		},
	}
}

func (s *BobaedreamScraper) Name() string { return s.communityName }

func (s *BobaedreamScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/list?code=best", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post

	table := doc.Find("table.clistTable02").First()
	if table.Length() == 0 {
		return posts, nil
	}

	table.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		aTag := tr.Find("td.pl14 a").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		votes := 0
		recTd := tr.Find("td.recomm").First()
		if recTd.Length() > 0 {
			text := strings.TrimSpace(strings.ReplaceAll(recTd.Text(), ",", ""))
			if v, err := strconv.Atoi(text); err == nil {
				votes = v
			}
		}

		views := 0
		cntTd := tr.Find("td.count").First()
		if cntTd.Length() > 0 {
			text := strings.TrimSpace(strings.ReplaceAll(cntTd.Text(), ",", ""))
			if v, err := strconv.Atoi(text); err == nil {
				views = v
			}
		}

		comments := 0
		replySpan := tr.Find("span.totreply").First()
		if replySpan.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(replySpan.Text())); err == nil {
				comments = v
			}
		}

		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		posts = append(posts, s.makePost(title, url, votes, views, comments))
	})

	return s.filterPosts(posts), nil
}
