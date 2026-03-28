package scraper

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

var (
	etolandNumRe  = regexp.MustCompile(`(\d[\d,]*)`)
	etolandGoodRe = regexp.MustCompile(`mw\.good\.php\?bo_table=(\w+)&wr_id=(\d+)`)
)

// EtolandScraper scrapes 이토랜드 hit posts (EUC-KR, URL resolution).
type EtolandScraper struct {
	baseScraper
}

func NewEtolandScraper() *EtolandScraper {
	return &EtolandScraper{
		baseScraper: baseScraper{
			community:     "etoland",
			communityName: "이토랜드",
			baseURL:       "https://www.etoland.co.kr",
			encoding:      "euc-kr",
		},
	}
}

func (s *EtolandScraper) Name() string { return s.communityName }

// resolveRealURL converts hit.php?bn_id= URL to board.php real URL.
func (s *EtolandScraper) resolveRealURL(client *http.Client, hitURL string) string {
	html, err := fetchHTML(client, hitURL, s.encoding, s.baseURL)
	if err != nil {
		return ""
	}
	m := etolandGoodRe.FindStringSubmatch(html)
	if m != nil {
		return fmt.Sprintf("%s/bbs/board.php?bo_table=%s&wr_id=%s", s.baseURL, m[1], m[2])
	}
	return ""
}

func (s *EtolandScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/bbs/hit.php", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	type rawPost struct {
		title    string
		hitURL   string
		votes    int
		views    int
		comments int
	}
	var rawPosts []rawPost
	seenBn := make(map[string]bool)

	doc.Find("li").Each(func(_ int, li *goquery.Selection) {
		classes, _ := li.Attr("class")
		if !strings.Contains(classes, "hit_item") {
			return
		}
		if strings.Contains(classes, "ad_list") {
			return
		}

		aTag := li.Find("a.content_link").First()
		if aTag.Length() == 0 {
			return
		}
		subj := li.Find("p.subject").First()
		if subj.Length() == 0 {
			return
		}
		title := strings.TrimSpace(subj.Text())
		href, exists := aTag.Attr("href")
		if !exists || title == "" || href == "" {
			return
		}

		// Votes
		votes := 0
		goodSpan := li.Find("span.good").First()
		if goodSpan.Length() > 0 {
			m := etolandNumRe.FindStringSubmatch(strings.TrimSpace(goodSpan.Text()))
			if m != nil {
				if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil {
					votes = v
				}
			}
		}

		// Views
		views := 0
		hitSpan := li.Find("span.hit").First()
		if hitSpan.Length() > 0 {
			m := etolandNumRe.FindStringSubmatch(strings.TrimSpace(hitSpan.Text()))
			if m != nil {
				if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil {
					views = v
				}
			}
		}

		// Comments
		comments := 0
		cmtSpan := li.Find("span.comment_cnt").First()
		if cmtSpan.Length() > 0 {
			m := etolandNumRe.FindStringSubmatch(strings.TrimSpace(cmtSpan.Text()))
			if m != nil {
				if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil {
					comments = v
				}
			}
		}

		// Resolve hit URL
		var hitURL string
		if strings.HasPrefix(href, "http") {
			hitURL = href
		} else if strings.HasPrefix(href, "/") {
			hitURL = s.baseURL + href
		} else {
			hitURL = s.baseURL + "/bbs/" + href
		}

		if !seenBn[hitURL] {
			seenBn[hitURL] = true
			rawPosts = append(rawPosts, rawPost{
				title:    title,
				hitURL:   hitURL,
				votes:    votes,
				views:    views,
				comments: comments,
			})
		}
	})

	// Resolve hit.php URLs to board.php URLs concurrently
	resolved := make([]string, len(rawPosts))
	var wg sync.WaitGroup
	for i, rp := range rawPosts {
		wg.Add(1)
		go func(idx int, hitURL string) {
			defer wg.Done()
			resolved[idx] = s.resolveRealURL(client, hitURL)
		}(i, rp.hitURL)
	}
	wg.Wait()

	var posts []Post
	seenURL := make(map[string]bool)
	for i, rp := range rawPosts {
		realURL := resolved[i]
		if realURL == "" {
			continue // resolution failed, skip
		}
		if !seenURL[realURL] {
			seenURL[realURL] = true
			posts = append(posts, s.makePost(rp.title, realURL, rp.votes, rp.views, rp.comments))
		}
	}

	return s.filterPosts(posts), nil
}
