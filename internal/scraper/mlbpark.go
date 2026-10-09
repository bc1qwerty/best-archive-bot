package scraper

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var mlbparkViewsRe = regexp.MustCompile(`조회\s*([\d,]+)`)

// MlbparkScraper scrapes MLB파크 best posts with individual page fetching.
type MlbparkScraper struct {
	baseScraper
}

func NewMlbparkScraper() *MlbparkScraper {
	return &MlbparkScraper{
		baseScraper: baseScraper{
			community:     "mlbpark",
			communityName: "MLB파크",
			baseURL:       "https://mlbpark.donga.com",
			encoding:      "utf-8",
			dataRequired:  true,
		},
	}
}

func (s *MlbparkScraper) Name() string { return s.communityName }

// parsePostDetail fetches an individual post page and extracts votes and views.
func (s *MlbparkScraper) parsePostDetail(client *http.Client, url string) (int, int) {
	doc, err := fetchDocument(client, url, s.encoding, s.baseURL)
	if err != nil {
		return 0, 0
	}

	votes := 0
	recoSpan := doc.Find("span.count_recommend").First()
	if recoSpan.Length() > 0 {
		text := strings.TrimSpace(strings.ReplaceAll(recoSpan.Text(), ",", ""))
		if v, err := strconv.Atoi(text); err == nil {
			votes = v
		}
	}

	views := 0
	text2 := doc.Find("div.text2").First()
	if text2.Length() > 0 {
		m := mlbparkViewsRe.FindStringSubmatch(text2.Text())
		if m != nil {
			if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil {
				views = v
			}
		}
	}

	return votes, views
}

// mlbparkTodayEmpty 는 TODAY BEST 가 비어 있는 정상 상태를 알아본다. 이 목록은
// KST 자정에 비워지고 30분쯤 채워지지 않는다. 그동안 표 자리에 안내 문구만
// 나오는데, 이를 0행 LayoutError 로 올려 매일 자정 error 경보가 떴다
// (2026-10-10 00:03 실물 = testdata/mlbpark-best-midnight.html).
// 표도 문구도 없으면 진짜 레이아웃 변경이므로 false 다.
func mlbparkTodayEmpty(doc *goquery.Document) bool {
	if doc.Find("table.tbl_type01").Length() > 0 {
		return false
	}
	return strings.Contains(doc.Find("div.message").Text(), "요청하신 정보가 존재하지 않습니다")
}

func (s *MlbparkScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/mp/best.php?b=bullpen&m=like", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}
	if mlbparkTodayEmpty(doc) {
		log.Printf("[%s] TODAY BEST 비어 있음 (자정 초기화)", s.communityName)
		return nil, nil
	}

	type entry struct {
		title string
		url   string
	}
	var entries []entry
	seen := make(map[string]bool)

	table := doc.Find("table.tbl_type01").First()
	tbody := table.Find("tbody").First()
	tbody.Find("tr").Each(func(_ int, tr *goquery.Selection) {
		tds := tr.Find("td")
		if tds.Length() < 4 {
			return
		}
		// Title is in the 2nd td (index 1)
		aTag := tds.Eq(1).Find("a").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || title == "" || href == "" {
			return
		}
		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		if !seen[url] {
			seen[url] = true
			entries = append(entries, entry{title: title, url: url})
		}
	})

	// Fetch individual pages concurrently (max 5 at a time)
	results := make([]Post, len(entries))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for i, e := range entries {
		wg.Add(1)
		go func(idx int, e entry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			votes, views := s.parsePostDetail(client, e.url)
			time.Sleep(300 * time.Millisecond)
			results[idx] = s.makePost(e.title, e.url, votes, views, 0)
		}(i, e)
	}
	wg.Wait()

	posts := make([]Post, 0, len(results))
	for _, p := range results {
		if p.URL != "" {
			posts = append(posts, p)
		}
	}

	log.Printf("[%s] %d entries, %d posts after individual fetch", s.communityName, len(entries), len(posts))
	return s.finish(posts)
}
