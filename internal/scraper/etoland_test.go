package scraper

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// 2026-10-01: 이토랜드가 Next.js 로 다시 지어져 예전 경로(`/bbs/hit.php`)가 308 로
// 홈페이지로 넘어갔다. 요청은 200 이고 목록만 0행이라 16일간 하루 ~100건씩
// `목록 셀렉터 0행` 만 적으며 조용히 죽어 있었다. 저장된 실물 응답으로 고정해
// **레이아웃이 또 바뀌면 GHA 가 먼저 빨개지게** 한다.
func loadEtolandFixture(t *testing.T) *goquery.Document {
	t.Helper()
	f, err := os.Open("testdata/etoland-hit.html")
	if err != nil {
		t.Fatalf("fixture 열기 실패: %v", err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("fixture 파싱 실패: %v", err)
	}
	return doc
}

func TestEtolandParseList(t *testing.T) {
	s := NewEtolandScraper()
	posts := s.parseList(loadEtolandFixture(t))

	if len(posts) == 0 {
		t.Fatal("0행 — 셀렉터가 새 레이아웃과 안 맞는다")
	}
	// fixture 는 실물에서 6행을 떼 온 것이다: 핫딜 1 + 스켈레톤(링크 없음) 1 +
	// 커뮤니티 글 4. 앞의 둘은 걸러져야 한다.
	if len(posts) != 4 {
		t.Errorf("행 수 %d, want 4 (핫딜·스켈레톤 제외)", len(posts))
	}

	for _, p := range posts {
		if p.Title == "" {
			t.Errorf("제목이 빈 행: %+v", p)
		}
		if !strings.HasPrefix(p.URL, "https://etoland.co.kr/hit/") {
			t.Errorf("URL 이 절대주소가 아니다: %q", p.URL)
		}
		if strings.Contains(p.URL, "/hit/hotdeal/") {
			t.Errorf("핫딜(쇼핑 특가)이 섞였다: %q", p.URL)
		}
		if p.CommunityName != "이토랜드" {
			t.Errorf("CommunityName=%q", p.CommunityName)
		}
	}

	// 댓글 수를 읽어야 이토랜드 기준(communityThresholds)이 걸린다. fixture 4행의
	// 실물 값이다 — 못 읽으면 FetchBestPosts 가 LayoutError 로 올린다.
	var got []int
	for _, p := range posts {
		got = append(got, p.Comments)
		if p.Votes != 0 || p.Views != 0 {
			t.Errorf("목록에 없는 지표가 채워졌다: %+v", p)
		}
	}
	if want := []int{14, 12, 4, 5}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("댓글 수 = %v, want %v", got, want)
	}
	if !anyComments(posts) {
		t.Error("anyComments=false — 레이아웃 경보가 오발한다")
	}
}

func TestEtolandThreshold(t *testing.T) {
	s := NewEtolandScraper()
	min := communityThresholds["etoland"].comments
	cases := []struct {
		comments int
		keep     bool
	}{{min, true}, {min - 1, false}, {0, false}} // 0 = 댓글 칸 없음, 면제로 통과하면 안 된다
	for _, c := range cases {
		p := s.makePost("t", "https://etoland.co.kr/hit/etohumor07/view/t-1", 0, 0, c.comments)
		if got := s.shouldInclude(p); got != c.keep {
			t.Errorf("comments=%d: shouldInclude=%v, want %v", c.comments, got, c.keep)
		}
	}
}

func TestEtolandBoardOf(t *testing.T) {
	ok := map[string]string{
		"/hit/etohumor07/view/제목-9467825": "etohumor07",
		"/hit/star02/view/x-1":            "star02",
		"/hit/hotdeal/view/특가-9468149":    "hotdeal",
	}
	for href, want := range ok {
		got, isPost := etolandBoardOf(href)
		if !isPost || got != want {
			t.Errorf("etolandBoardOf(%q) = (%q,%v), want (%q,true)", href, got, isPost, want)
		}
	}
	// 목록 페이지에 섞여 있는 메뉴·배너·구버전 링크는 전부 걸러져야 한다.
	for _, href := range []string{
		"/hit/list", "/hit/list?filter=latest", "/b/freebbs/list", "/",
		"/bbs/hit.php", "/hit/etohumor07/", "/hit//view/x", "/hit/star02/view/",
		"https://etoland.co.kr/hit/star02/view/x-1", // 절대주소는 이 목록에 안 나온다
	} {
		if got, isPost := etolandBoardOf(href); isPost {
			t.Errorf("etolandBoardOf(%q) = (%q,true), want false", href, got)
		}
	}
}

// 구버전으로 되돌아가면 깨지는 가드. 이 셋이 한 번에 틀렸던 것이 16일 무음의 원인이다.
func TestEtolandUsesRebuiltSite(t *testing.T) {
	s := NewEtolandScraper()
	if strings.Contains(s.baseURL, "www.") {
		t.Errorf("baseURL=%q — www 는 경로를 버리고 홈으로 308 한다", s.baseURL)
	}
	if !strings.EqualFold(s.encoding, "utf-8") {
		t.Errorf("encoding=%q — 새 사이트는 UTF-8 이다", s.encoding)
	}
	src, err := os.ReadFile("etoland.go")
	if err != nil {
		t.Fatalf("소스 읽기 실패: %v", err)
	}
	// ⚠주석에는 남아 있어야 한다(왜 바뀌었는지가 거기 적혀 있다). 코드의 문자열만 본다.
	if strings.Contains(string(src), `/bbs/hit.php"`) {
		t.Error("사라진 경로 /bbs/hit.php 가 아직 코드에 남아 있다")
	}
	if !strings.Contains(string(src), `"/hit/list"`) {
		t.Error("새 목록 경로 /hit/list 가 없다")
	}
}
