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

var cook82ViewsRe = regexp.MustCompile(`조회수\s*:\s*([\d,]+)`)

// Cook82Scraper scrapes 82cook best posts with individual page fetching for views.
type Cook82Scraper struct {
	baseScraper
}

func NewCook82Scraper() *Cook82Scraper {
	return &Cook82Scraper{
		baseScraper: baseScraper{
			community:     "cook82",
			communityName: "82cook",
			baseURL:       "https://www.82cook.com",
			encoding:      "utf-8",
			dataRequired:  true,
		},
	}
}

func (s *Cook82Scraper) Name() string { return s.communityName }

// parseViews fetches an individual post page and extracts the view count.
func (s *Cook82Scraper) parseViews(client *http.Client, url string) int {
	doc, err := fetchDocument(client, url, s.encoding, s.baseURL)
	if err != nil {
		return 0
	}
	rl := doc.Find("div.readLeft").First()
	if rl.Length() == 0 {
		return 0
	}
	m := cook82ViewsRe.FindStringSubmatch(rl.Text())
	if m == nil {
		return 0
	}
	v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		return 0
	}
	return v
}

func (s *Cook82Scraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/entiz/enti.php?bn=15", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	type entry struct {
		title string
		url   string
	}
	var entries []entry

	doc.Find("ul.most li a").Each(func(_ int, aTag *goquery.Selection) {
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}
		url := href
		if !strings.HasPrefix(href, "http") {
			url = s.baseURL + href
		}
		entries = append(entries, entry{title: title, url: url})
	})

	// Fetch individual pages concurrently (max 5 at a time)
	type result struct {
		idx   int
		views int
	}
	results := make([]Post, len(entries))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for i, e := range entries {
		wg.Add(1)
		go func(idx int, e entry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			views := s.parseViews(client, e.url)
			time.Sleep(300 * time.Millisecond)
			results[idx] = s.makePost(e.title, e.url, 0, views, 0)
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
	return s.filterPosts(posts), nil
}
