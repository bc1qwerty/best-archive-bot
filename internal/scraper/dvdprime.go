package scraper

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var dvdprimeNumRe = regexp.MustCompile(`(\d[\d,]*)`)

// metricMap maps section title keywords to Post field names.
var metricMap = map[string]string{
	"추천": "votes",
	"댓글": "comments",
	"조회": "views",
}

// DvdprimeScraper scrapes DVD프라임 community best posts.
type DvdprimeScraper struct {
	baseScraper
}

func NewDvdprimeScraper() *DvdprimeScraper {
	return &DvdprimeScraper{
		baseScraper: baseScraper{
			community:     "dvdprime",
			communityName: "DVD프라임",
			baseURL:       "https://dvdprime.com",
			encoding:      "utf-8",
		},
	}
}

func (s *DvdprimeScraper) Name() string { return s.communityName }

func (s *DvdprimeScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	pageURL := fmt.Sprintf("%s/g2/bbs/board.php?bo_table=comm", s.baseURL)
	doc, err := fetchDocument(client, pageURL, s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	seen := make(map[string]bool)

	doc.Find("div.bottom_row_third").Each(func(_ int, box *goquery.Selection) {
		// Determine metric type from section title
		titleDiv := box.Find("div.bottom_row_title").First()
		if titleDiv.Length() == 0 {
			return
		}
		sectionText := strings.TrimSpace(titleDiv.Text())
		var metric string
		for keyword, field := range metricMap {
			if strings.Contains(sectionText, keyword) {
				metric = field
				break
			}
		}
		if metric == "" {
			return
		}

		box.Find("div.rc_box_list_row_inner").Each(func(_ int, row *goquery.Selection) {
			right := row.Find("div.rc_box_best_right").First()
			if right.Length() == 0 {
				return
			}
			aTag := right.Find("a").First()
			if aTag.Length() == 0 {
				return
			}
			title := strings.TrimSpace(aTag.Text())
			href, exists := aTag.Attr("href")
			if !exists || title == "" || href == "" {
				return
			}

			// Parse count value
			count := 0
			right2 := row.Find("div.rc_box_best_right2").First()
			if right2.Length() > 0 {
				m := dvdprimeNumRe.FindStringSubmatch(strings.TrimSpace(right2.Text()))
				if m != nil {
					if v, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", "")); err == nil {
						count = v
					}
				}
			}

			// Resolve relative URL
			resolved := href
			if !strings.HasPrefix(href, "http") {
				base, _ := url.Parse(pageURL)
				ref, _ := url.Parse(href)
				resolved = base.ResolveReference(ref).String()
			}

			if !seen[resolved] {
				seen[resolved] = true
				var votes, views, comments int
				switch metric {
				case "votes":
					votes = count
				case "views":
					views = count
				case "comments":
					comments = count
				}
				posts = append(posts, s.makePost(title, resolved, votes, views, comments))
			}
		})
	})

	return s.filterPosts(posts), nil
}
