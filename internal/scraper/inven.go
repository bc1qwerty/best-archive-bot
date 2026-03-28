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

var (
	invenManRe    = regexp.MustCompile(`([\d.]+)만`)
	invenDigitRe  = regexp.MustCompile(`\d+`)
)

// InvenScraper scrapes 인벤 hot posts.
type InvenScraper struct {
	baseScraper
}

func NewInvenScraper() *InvenScraper {
	return &InvenScraper{
		baseScraper: baseScraper{
			community:     "inven",
			communityName: "인벤",
			baseURL:       "https://www.inven.co.kr",
			encoding:      "utf-8",
		},
	}
}

func (s *InvenScraper) Name() string { return s.communityName }

// parseInvenViews parses views like "1.1만" → 11000, "5,559" → 5559.
func parseInvenViews(text string) int {
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", ""))
	m := invenManRe.FindStringSubmatch(text)
	if m != nil {
		f, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			return int(math.Round(f * 10000))
		}
	}
	v, err := strconv.Atoi(text)
	if err != nil {
		return 0
	}
	return v
}

func (s *InvenScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, fmt.Sprintf("%s/hot/", s.baseURL), s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}

	var posts []Post
	doc.Find("div.list-common").Each(func(_ int, item *goquery.Selection) {
		// Must have "con" class
		if classes, exists := item.Attr("class"); exists {
			if !strings.Contains(classes, "con") {
				return
			}
		} else {
			return
		}

		// Title link
		titleDiv := item.Find("div.title").First()
		if titleDiv.Length() == 0 {
			return
		}
		aTag := titleDiv.Find("a").First()
		if aTag.Length() == 0 {
			return
		}
		href, exists := aTag.Attr("href")
		if !exists || href == "" {
			return
		}

		// Title: from name div, strip num and cate prefixes
		nameDiv := item.Find("div.name").First()
		if nameDiv.Length() == 0 {
			return
		}
		title := strings.TrimSpace(nameDiv.Text())

		numDiv := item.Find("div.num").First()
		if numDiv.Length() > 0 {
			numText := strings.TrimSpace(numDiv.Text())
			if numText != "" && strings.HasPrefix(title, numText) {
				title = title[len(numText):]
			}
		}
		cateDiv := item.Find("div.cate").First()
		if cateDiv.Length() > 0 {
			cateText := strings.TrimSpace(cateDiv.Text())
			if cateText != "" && strings.HasPrefix(title, cateText) {
				title = title[len(cateText):]
			}
		}
		title = strings.TrimSpace(title)
		if title == "" {
			return
		}

		// Votes
		votes := 0
		recoDiv := item.Find("div.reco").First()
		if recoDiv.Length() > 0 {
			if v, err := strconv.Atoi(strings.TrimSpace(recoDiv.Text())); err == nil {
				votes = v
			}
		}

		// Views
		views := 0
		hitsDiv := item.Find("div.hits").First()
		if hitsDiv.Length() > 0 {
			views = parseInvenViews(hitsDiv.Text())
		}

		// Comments
		comments := 0
		cmtDiv := item.Find("div.comment").First()
		if cmtDiv.Length() > 0 {
			m := invenDigitRe.FindString(cmtDiv.Text())
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
		posts = append(posts, s.makePost(title, url, votes, views, comments))
	})

	return s.filterPosts(posts), nil
}
