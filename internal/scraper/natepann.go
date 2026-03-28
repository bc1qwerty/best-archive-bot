package scraper

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var natepannDigitRe = regexp.MustCompile(`\d+`)
var natepannFallbackRe = regexp.MustCompile(`^/talk/\d+`)

// NatepannScraper scrapes 네이트판 ranking.
type NatepannScraper struct {
	baseScraper
}

func NewNatepannScraper() *NatepannScraper {
	return &NatepannScraper{
		baseScraper: baseScraper{
			community:     "natepann",
			communityName: "네이트판",
			baseURL:       "https://pann.nate.com",
			encoding:      "utf-8",
			dataRequired:  true,
		},
	}
}

func (s *NatepannScraper) Name() string { return s.communityName }

func (s *NatepannScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/talk/ranking", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post

	doc.Find("ul.post_wrap > li").Each(func(_ int, li *goquery.Selection) {
		aTag := li.Find("h2 a[href]").First()
		if aTag.Length() == 0 {
			return
		}
		title := strings.TrimSpace(aTag.Text())
		href, exists := aTag.Attr("href")
		if !exists || title == "" || href == "" {
			return
		}

		votes := 0
		rcm := li.Find("span.rcm").First()
		if rcm.Length() > 0 {
			m := natepannDigitRe.FindString(rcm.Text())
			if m != "" {
				if v, err := strconv.Atoi(m); err == nil {
					votes = v
				}
			}
		}

		comments := 0
		reple := li.Find("span.reple-num").First()
		if reple.Length() > 0 {
			m := natepannDigitRe.FindString(reple.Text())
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

	// Fallback: broader selector
	if len(posts) == 0 {
		doc.Find("a[href]").Each(func(_ int, aTag *goquery.Selection) {
			href, _ := aTag.Attr("href")
			if !natepannFallbackRe.MatchString(href) {
				return
			}
			// Check parent is h2
			parent := aTag.Parent()
			if parent.Length() == 0 || goquery.NodeName(parent) != "h2" {
				return
			}
			title := strings.TrimSpace(aTag.Text())
			if title == "" {
				return
			}
			url := s.baseURL + href
			posts = append(posts, s.makePost(title, url, 0, 0, 0))
		})
	}

	return s.filterPosts(posts), nil
}
