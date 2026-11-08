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

// bobaedreamServedEmpty 는 보배드림이 일부 IP 에 보내는 «목록만 비운» 페이지를 알아본다.
// 2026-11-07 실측: 제목·메뉴·표 틀·머리글은 정상인데 `<tbody>` 만 비어 온다(72,850B,
// 정상 121,516B). GHA 러너 IP 에 따라 갈리고(같은 러너면 매번 같다, curl 도 같다)
// 다른 IP 는 403 을 받는다. 레이아웃 변경이 아니라 차단이라 LayoutError 로 올리지 않는다
// (testdata/bobaedream-empty-tbody.html). 표 자체가 없으면 그대로 0행 LayoutError 다.
func bobaedreamServedEmpty(doc *goquery.Document) bool {
	table := doc.Find("table.clistTable02").First()
	return table.Find("thead th").Length() > 0 && table.Find("tbody tr").Length() == 0
}

func (s *BobaedreamScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/list?code=best", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	if bobaedreamServedEmpty(doc) {
		return nil, fmt.Errorf("%s: 목록 본문이 빈 페이지(표 틀만 옴 — IP 차단 추정)", s.communityName)
	}

	var posts []Post

	table := doc.Find("table.clistTable02").First()
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
		replySpan := tr.Find(".totreply").First()
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

	return s.finish(posts)
}
