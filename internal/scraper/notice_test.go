package scraper

import "testing"

func TestIsNoticeTitle(t *testing.T) {
	notices := []string{
		"[공지] 서버 점검 안내",
		"[ 공지 ] 띄어쓰기 포함",
		"【공지】 이벤트",
		"[필독] 규칙",
		"[이벤트] 출석 체크",
		"실시간베스트 갤러리 이용 안내",
		"커뮤니티 운영원칙 변경",
	}
	for _, tt := range notices {
		if !isNoticeTitle(tt) {
			t.Errorf("expected notice: %q", tt)
		}
	}

	posts := []string{
		"[싱갤] 싱글벙글 9월 개봉예정 영화",
		"[유머] 회사 공지 실화냐 ㅋㅋ", // "공지" mid-title must NOT match
		"가수 청하의 외할아버지",
		"[국갤] 중국 군인 출신 간첩들 구속",
	}
	for _, tt := range posts {
		if isNoticeTitle(tt) {
			t.Errorf("false positive (real post flagged): %q", tt)
		}
	}
}

func TestIsAllDigits(t *testing.T) {
	yes := []string{"1", "453166", "007"}
	no := []string{"", "공지", "설문", "AD", "12a", " 12"}
	for _, s := range yes {
		if !isAllDigits(s) {
			t.Errorf("expected all-digits: %q", s)
		}
	}
	for _, s := range no {
		if isAllDigits(s) {
			t.Errorf("expected NOT all-digits: %q", s)
		}
	}
}

func TestFinish_ZeroRawRowsIsError(t *testing.T) {
	b := &baseScraper{community: "test", communityName: "테스트"}

	// 목록 셀렉터가 0행 — 레이아웃 변경/안티봇 페이지는 오류로 떠야 한다.
	if _, err := b.finish(nil); err == nil {
		t.Error("finish(nil) = nil error, want layout-change error")
	}

	// 행은 잡혔지만 필터가 전부 걸러낸 것은 정상적인 빈 결과다.
	posts := []Post{{Title: "t", URL: "not-a-link"}}
	out, err := b.finish(posts)
	if err != nil {
		t.Errorf("finish(filtered-to-zero) error: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("got %d posts, want 0", len(out))
	}
}
