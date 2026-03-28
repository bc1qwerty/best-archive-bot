package scraper

import (
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var clienKRe = regexp.MustCompile(`(?i)([\d.]+)\s*k`)

// ClienScraper scrapes 클리앙 best posts.
type ClienScraper struct {
	baseScraper
}

func NewClienScraper() *ClienScraper {
	return &ClienScraper{
		baseScraper: baseScraper{
			community:     "clien",
			communityName: "클리앙",
			baseURL:       "https://www.clien.net",
			encoding:      "utf-8",
		},
	}
}

func (s *ClienScraper) Name() string { return s.communityName }

// parseHit parses view counts like "13.8 k" → 13800, "607" → 607.
func parseHit(text string) int {
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	m := clienKRe.FindStringSubmatch(text)
	if m != nil {
		f, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return int(math.Round(f * 1000))
		}
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return v
}

func (s *ClienScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/service/group/clien_all?&od=T33", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	doc.Find("div.list_item").Each(func(_ int, item *goquery.Selection) {
		// Skip notices
		if classes, exists := item.Attr("class"); exists {
			if strings.Contains(classes, "notice") {
				return
			}
		}

		aTag := item.Find("a.list_subject").First()
		if aTag.Length() == 0 {
			return
		}
		titleSpan := aTag.Find("span.subject_fixed")
		var title string
		if titleSpan.Length() > 0 {
			title = strings.TrimSpace(titleSpan.Text())
		} else {
			title = strings.TrimSpace(aTag.Text())
		}
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		votes := 0
		symph := item.Find("div.list_symph span").First()
		if symph.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(symph.Text())); err == nil {
				votes = v
			}
		}

		views := 0
		hitSpan := item.Find("div.list_hit span.hit").First()
		if hitSpan.Length() > 0 {
			views = parseHit(hitSpan.Text())
		}

		comments := 0
		replySpan := item.Find("a.list_reply span").First()
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
