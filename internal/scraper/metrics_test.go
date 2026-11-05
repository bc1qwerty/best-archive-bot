package scraper

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"golang.org/x/text/encoding/korean"
)

// 2026-11-06: 목록에 있는 지표를 여럿이 안 읽고 있었고, 보배드림 댓글은 선택자가
// 틀려(`span.totreply` ↔ 실제 `strong.totreply`) 늘 0 이었다. 지표 하나가 조용히
// 0 이 되면 그 기준은 그냥 꺼진다. 실물 목록에서 한 행씩 떼어 고정한다.
func TestListMetrics(t *testing.T) {
	config.RequestDelayMin, config.RequestDelayMax = 0, 0
	// 파싱만 본다 — 기준은 전부 1 로 낮춰 행이 걸러지지 않게.
	saved := communityThresholds
	communityThresholds = map[string]thresholds{}
	for _, s := range AllScrapers() {
		communityThresholds[s.(interface{ base() *baseScraper }).base().community] = thresholds{1, 1, 1}
	}
	defer func() { communityThresholds = saved }()

	cases := []struct {
		fixture string
		scraper func(base string) Scraper
		want    [3]int // 추천, 조회, 댓글
	}{
		{"bobaedream", func(b string) Scraper { s := NewBobaedreamScraper(); s.baseURL = b; return s }, [3]int{18, 1819, 12}},
		{"ruliweb", func(b string) Scraper { s := NewRuliwebScraper(); s.baseURL = b; return s }, [3]int{18, 6006, 35}},
		{"natepann", func(b string) Scraper { s := NewNatepannScraper(); s.baseURL = b; return s }, [3]int{27, 15481, 42}},
		{"ppomppu", func(b string) Scraper { s := NewPpomppuScraper(); s.baseURL = b; return s }, [3]int{115, 6564, 11}},
		{"dcinside", func(b string) Scraper { s := NewDcinsideScraper(); s.baseURL = b; return s }, [3]int{5, 524, 23}},
		{"theqoo", func(b string) Scraper { s := NewTheqooScraper(); s.baseURL = b; return s }, [3]int{0, 6687, 123}},
		{"humoruniv", func(b string) Scraper { s := NewHumorunivScraper(); s.baseURL = b; return s }, [3]int{1979, 68864, 152}},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			body, err := os.ReadFile("testdata/rows/" + c.fixture + ".html")
			if err != nil {
				t.Fatal(err)
			}
			s := c.scraper("")
			if s.(interface{ base() *baseScraper }).base().encoding == "euc-kr" {
				if body, err = korean.EUCKR.NewEncoder().Bytes(body); err != nil {
					t.Fatal(err)
				}
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
			defer srv.Close()

			posts, err := c.scraper(srv.URL).FetchBestPosts(srv.Client())
			if err != nil || len(posts) != 1 {
				t.Fatalf("posts=%d err=%v", len(posts), err)
			}
			p := posts[0]
			if got := [3]int{p.Votes, p.Views, p.Comments}; got != c.want {
				t.Errorf("지표(추천,조회,댓글) = %v, want %v (제목 %q)", got, c.want, p.Title)
			}
		})
	}
}
