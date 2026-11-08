package scraper

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
)

func serveFixture(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv
}

// 일부 IP 가 받는 «목록만 비운» 페이지는 차단(일반 오류)이지 레이아웃 변경이 아니다.
func TestBobaedreamEmptyTbodyIsNotLayoutError(t *testing.T) {
	config.RequestDelayMin, config.RequestDelayMax = 0, 0
	body, err := os.ReadFile("testdata/bobaedream-empty-tbody.html")
	if err != nil {
		t.Fatal(err)
	}
	srv := serveFixture(t, body)
	s := NewBobaedreamScraper()
	s.baseURL = srv.URL
	posts, err := s.FetchBestPosts(srv.Client())
	if err == nil || len(posts) != 0 {
		t.Fatalf("posts=%d err=%v, want 오류", len(posts), err)
	}
	var layout *LayoutError
	if errors.As(err, &layout) {
		t.Fatalf("빈 목록 페이지가 LayoutError 로 올라갔다: %v", err)
	}
}

// 표 자체가 없으면 지금처럼 LayoutError 여야 한다(진짜 레이아웃 변경).
func TestBobaedreamNoTableIsLayoutError(t *testing.T) {
	config.RequestDelayMin, config.RequestDelayMax = 0, 0
	srv := serveFixture(t, []byte("<html><head><title>x</title></head><body><div>점검 중</div></body></html>"))
	s := NewBobaedreamScraper()
	s.baseURL = srv.URL
	_, err := s.FetchBestPosts(srv.Client())
	var layout *LayoutError
	if !errors.As(err, &layout) {
		t.Fatalf("err=%v, want LayoutError", err)
	}
}
