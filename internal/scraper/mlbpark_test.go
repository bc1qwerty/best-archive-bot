package scraper

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestMlbparkTodayEmpty(t *testing.T) {
	f, err := os.Open("testdata/mlbpark-best-midnight.html")
	if err != nil {
		t.Fatalf("fixture 열기 실패: %v", err)
	}
	defer f.Close()
	midnight, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("fixture 파싱 실패: %v", err)
	}
	if !mlbparkTodayEmpty(midnight) {
		t.Error("자정 실물 응답을 빈 목록으로 알아보지 못했다")
	}

	cases := map[string]string{
		"표가 있으면 비어 있지 않다":    `<table class="tbl_type01"><tbody></tbody></table><div class='message'>요청하신 정보가 존재하지 않습니다.</div>`,
		"표도 문구도 없으면 레이아웃 변경": `<div class="new_layout"></div>`,
	}
	for name, html := range cases {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		if err != nil {
			t.Fatal(err)
		}
		if mlbparkTodayEmpty(doc) {
			t.Errorf("%s: 빈 목록으로 오판했다", name)
		}
	}
}
