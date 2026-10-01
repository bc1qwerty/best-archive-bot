package scraper

import (
	"net/http"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// EtolandScraper scrapes 이토랜드 인기글(hit) 목록.
//
// ⚠2026-10-01 전면 재작성. 사이트가 Next.js 로 다시 지어져 **예전 경로가 사라졌다**:
//
//	https://www.etoland.co.kr/bbs/hit.php  →  308  →  https://etoland.co.kr/  (홈)
//
// 리다이렉트 끝이 홈페이지라 fetch 는 **HTTP 200 으로 성공**하고, 그 홈페이지에
// `li.hit_item` 이 없어 목록이 0행이 됐다. 즉 «성공한 요청 + 빈 목록» 이라
// 2026-09-16 부터 16일간 하루 ~100건씩 `목록 셀렉터 0행` 만 적으며 조용히 죽어 있었다
// (허브 로그 실측). 그동안 이 커뮤니티의 글은 한 건도 수집되지 않았다.
//
// 새 구조에서 달라진 것:
//   - 목록 URL 은 `https://etoland.co.kr/hit/list` (apex. www 는 경로를 버리고 홈으로 308)
//   - 인코딩이 **EUC-KR → UTF-8**
//   - 글 URL 이 이미 최종 주소다 → 예전의 `hit.php?bn_id=` → `board.php` 2단 해석이 불필요
//     (요청 수가 행당 1건씩 줄어든다)
//   - 목록에 **추천·조회가 실리지 않는다.** 댓글 수만 있는데 자릿수가 다르다
//     (실측 69행: 최소 4 · 중앙 20 · 최대 111). `config.MinComments` 는 150 이라
//     그 값으로 인기 판정을 하면 **전량이 걸러져 또 조용한 0행**이 된다.
//     그래서 지표를 채우지 않는다 — `/hit/list` 자체가 사이트의 인기글 선별이고,
//     지표가 없으면 `shouldInclude` 가 면제로 통과시킨다(`dataRequired` 는 false).
type EtolandScraper struct {
	baseScraper
}

func NewEtolandScraper() *EtolandScraper {
	return &EtolandScraper{
		baseScraper: baseScraper{
			community:     "etoland",
			communityName: "이토랜드",
			baseURL:       "https://etoland.co.kr",
			encoding:      "utf-8",
		},
	}
}

func (s *EtolandScraper) Name() string { return s.communityName }

func (s *EtolandScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, s.baseURL+"/hit/list", s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}
	return s.finish(s.parseList(doc))
}

// parseList 는 문서에서 인기글 행을 뽑는다. fetch 와 분리해 둔 것은 저장된
// 실물 응답(`testdata/etoland-hit.html`)으로 시험하기 위한 것이다 — 레이아웃이
// 또 바뀌면 시험이 먼저 깨져야 한다.
func (s *EtolandScraper) parseList(doc *goquery.Document) []Post {
	var posts []Post
	seen := make(map[string]bool)

	doc.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		board, ok := etolandBoardOf(href)
		if !ok {
			return
		}
		// 핫딜은 커뮤니티 글이 아니라 쇼핑 특가다(예전 목록의 `ad_list` 자리).
		if board == "hotdeal" {
			return
		}
		// 제목은 첫 `span.truncate` 다. 클라이언트 렌더 전 스켈레톤 행에는 링크
		// 자체가 없지만, 제목이 빈 행은 여기서도 버린다.
		title := strings.TrimSpace(a.Find("span.truncate").First().Text())
		if title == "" {
			return
		}
		url := s.baseURL + href
		if seen[url] {
			return
		}
		seen[url] = true
		// ⚠지표는 일부러 0 이다(위 주석). makePost 의 인자 순서: votes, views, comments.
		posts = append(posts, s.makePost(title, url, 0, 0, 0))
	})

	return posts
}

// etolandBoardOf 는 `/hit/<board>/view/<slug>` 에서 board 를 꺼낸다.
// 목록 페이지에는 메뉴·배너 링크가 섞여 있으므로 모양으로 걸러야 한다.
func etolandBoardOf(href string) (string, bool) {
	const prefix = "/hit/"
	if !strings.HasPrefix(href, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(href, prefix)
	board, after, found := strings.Cut(rest, "/")
	if !found || board == "" || !strings.HasPrefix(after, "view/") || len(after) <= len("view/") {
		return "", false
	}
	return board, true
}
