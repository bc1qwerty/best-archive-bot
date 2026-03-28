package scraper

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var ppomppuVoteRe = regexp.MustCompile(`(\d+)\s*-\s*\d+`)

// PpomppuScraper scrapes 뽐뿌 hot posts (EUC-KR).
type PpomppuScraper struct {
	baseScraper
}

func NewPpomppuScraper() *PpomppuScraper {
	return &PpomppuScraper{
		baseScraper: baseScraper{
			community:     "ppomppu",
			communityName: "뽐뿌",
			baseURL:       "https://www.ppomppu.co.kr",
			encoding:      "euc-kr",
		},
	}
}

func (s *PpomppuScraper) Name() string { return s.communityName }

func (s *PpomppuScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/hot.php", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	seen := make(map[string]bool)

	doc.Find("tr.baseList.bbs_new1").Each(func(_ int, row *goquery.Selection) {
		// Skip ad/sponsored categories
		tds := row.Find("td")
		if tds.Length() > 0 {
			cat := strings.TrimSpace(tds.First().Text())
			switch cat {
			case "렌탈업체", "뽐뿌스폰서", "가전견적상담", "휴대폰업체":
				return
			}
		}

		// Find the text-bearing a.baseList-title
		var aTag *goquery.Selection
		row.Find("a.baseList-title").Each(func(_ int, a *goquery.Selection) {
			if strings.TrimSpace(a.Text()) != "" && aTag == nil {
				aTag = a
			}
		})
		if aTag == nil {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || href == "" || title == "" {
			return
		}

		// Skip AD posts
		if strings.HasPrefix(title, "AD") {
			return
		}

		// Parse votes: "N - N" pattern
		votes := 0
		views := 0
		spaceTds := row.Find("td.baseList-space")
		spaceTds.Each(func(_ int, td *goquery.Selection) {
			text := strings.TrimSpace(td.Text())
			m := ppomppuVoteRe.FindStringSubmatch(text)
			if m != nil && votes == 0 {
				if v, err := strconv.Atoi(m[1]); err == nil {
					votes = v
				}
			}
		})

		// Views: last td.baseList-space
		if spaceTds.Length() > 0 {
			lastTd := spaceTds.Last()
			viewsText := strings.TrimSpace(strings.ReplaceAll(lastTd.Text(), ",", ""))
			if v, err := strconv.Atoi(viewsText); err == nil {
				views = v
			}
		}

		var url string
		if strings.HasPrefix(href, "http") {
			url = href
		} else if strings.HasPrefix(href, "/") {
			url = s.baseURL + href
		} else {
			url = s.baseURL + "/" + href
		}

		// Deduplicate by URL
		if !seen[url] {
			seen[url] = true
			posts = append(posts, s.makePost(title, url, votes, views, 0))
		}
	})

	return s.filterPosts(posts), nil
}
