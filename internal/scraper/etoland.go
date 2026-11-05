package scraper

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var (
	etolandCommentRe = regexp.MustCompile(`\((\d+)\)`)
	etolandStatRe    = regexp.MustCompile(`^(조회|추천)\s*([\d,]+)$`)
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
//   - 지표는 제목 옆 `(댓글)` 과 아랫줄 `조회 N | 추천 N` 에 있다. 자릿수가 다른
//     사이트보다 작아 전역 기준으로는 전량이 걸러지므로 `communityThresholds` 에
//     이토랜드 몫을 따로 둔다.
//
// 2026-11-06: 지표가 없다고 잘못 보고 전량 면제로 통과시키던 동안 하루 218건,
// 전체의 57%를 차지했다. 조회수는 모든 행에 있으므로, 한 행도 못 읽으면 전량이
// 조용히 걸러지기 전에 레이아웃 오류로 올린다.
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
			dataRequired:  true, // 지표를 못 읽은 행을 «지표 없음» 면제로 통과시키지 않는다
		},
	}
}

func (s *EtolandScraper) Name() string { return s.communityName }

func (s *EtolandScraper) FetchBestPosts(client *http.Client) ([]Post, error) {
	doc, err := fetchDocument(client, s.baseURL+"/hit/list", s.encoding, s.baseURL)
	if err != nil {
		return nil, err
	}
	posts := s.parseList(doc)
	if len(posts) > 0 && !anyViews(posts) {
		return nil, &LayoutError{Community: s.communityName, Reason: "조회수를 한 행도 못 읽었다 — 레이아웃 변경 의심"}
	}
	return s.finish(posts)
}

func anyViews(posts []Post) bool {
	for _, p := range posts {
		if p.Views > 0 {
			return true
		}
	}
	return false
}

// parseList 는 문서에서 인기글 행을 뽑는다. fetch 와 분리해 둔 것은 저장된
// 실물 응답(`testdata/etoland-hit.html`)으로 시험하기 위한 것이다 — 레이아웃이
// 또 바뀌면 시험이 먼저 깨져야 한다.
func (s *EtolandScraper) parseList(doc *goquery.Document) []Post {
	var posts []Post
	seen := make(map[string]int) // url → posts 의 위치

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
		// 댓글 수는 `(<!-- -->12<!-- -->)` 모양이다. 댓글이 없는 행엔 칸이 없다.
		comments := 0
		if m := etolandCommentRe.FindStringSubmatch(a.Find("span.comment-s").First().Text()); m != nil {
			comments, _ = strconv.Atoi(m[1])
		}
		votes, views := 0, 0
		a.Find("div.caption-m span").Each(func(_ int, sp *goquery.Selection) {
			if m := etolandStatRe.FindStringSubmatch(strings.TrimSpace(sp.Text())); m != nil {
				n, _ := strconv.Atoi(strings.ReplaceAll(m[2], ",", ""))
				if m[1] == "조회" {
					views = n
				} else {
					votes = n
				}
			}
		})
		// 같은 글이 사이드바 순위 위젯(댓글 수만 있다)과 본문 목록에 둘 다 나온다.
		// 먼저 나온 쪽이 이기면 조회·추천이 0 이 되므로 지표를 합친다.
		if i, dup := seen[url]; dup {
			p := &posts[i]
			p.Votes, p.Views, p.Comments = max(p.Votes, votes), max(p.Views, views), max(p.Comments, comments)
			return
		}
		seen[url] = len(posts)
		posts = append(posts, s.makePost(title, url, votes, views, comments))
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
