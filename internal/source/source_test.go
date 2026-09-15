package source

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
	"github.com/bc1qwerty/txid-bot-framework/pkg/core"
)

func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strip page and fragment, keep id/no",
			in:   "https://gall.dcinside.com/board/view/?id=dcbest&no=123&page=4#comment",
			want: "https://gall.dcinside.com/board/view/?id=dcbest&no=123",
		},
		{
			name: "strip utm tracking",
			in:   "https://example.com/post/999?utm_source=x&utm_medium=y",
			want: "https://example.com/post/999",
		},
		{
			name: "no query untouched",
			in:   "https://example.com/a/b",
			want: "https://example.com/a/b",
		},
		{
			name: "keep non-volatile params",
			in:   "https://example.com/v?bo=free&no=7",
			want: "https://example.com/v?bo=free&no=7",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeURL(c.in); got != c.want {
				t.Errorf("NormalizeURL(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNormalizeTitleKey(t *testing.T) {
	// Bracket tags, spacing, and punctuation differences must collapse.
	a := normalizeTitleKey("[더쿠] 인기글  가수 청하!!")
	b := normalizeTitleKey("가수 청하")
	if normalizeTitleKey("가수 청하") == "" {
		t.Fatal("expected non-empty key for hangul title")
	}
	if a == b {
		t.Errorf("distinct headlines collapsed: %q vs %q", a, b)
	}
	// Same headline with different decoration collapses.
	x := normalizeTitleKey("배우 하영 ㅁㅊ")
	y := normalizeTitleKey("배우 하영, ㅁㅊ!")
	if x != y {
		t.Errorf("same headline did not collapse: %q vs %q", x, y)
	}
}

func TestSortByPopularity(t *testing.T) {
	posts := []scraper.Post{
		{Title: "low", Votes: 10},
		{Title: "high", Votes: 900},
		{Title: "mid", Votes: 300},
	}
	sortByPopularity(posts)
	got := []string{posts[0].Title, posts[1].Title, posts[2].Title}
	want := []string{"high", "mid", "low"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestSortByPopularity_ZeroMetricsStable(t *testing.T) {
	posts := []scraper.Post{{Title: "a"}, {Title: "b"}, {Title: "c"}}
	sortByPopularity(posts)
	if posts[0].Title != "a" || posts[1].Title != "b" || posts[2].Title != "c" {
		t.Errorf("zero-metric order not stable: %v", posts)
	}
}

type fakeSource struct {
	name  string
	items []core.Item
	err   error
}

func (f fakeSource) Name() string { return f.name }
func (f fakeSource) Fetch(context.Context) ([]core.Item, error) {
	return f.items, f.err
}

func TestInterleave_MaxPerSourceCaps(t *testing.T) {
	// Source A is popularity-sorted upstream, so items are hottest-first.
	a := fakeSource{name: "A", items: []core.Item{
		{ID: "a1", Title: "A1"}, {ID: "a2", Title: "A2"}, {ID: "a3", Title: "A3"},
		{ID: "a4", Title: "A4"}, {ID: "a5", Title: "A5"},
	}}
	b := fakeSource{name: "B", items: []core.Item{
		{ID: "b1", Title: "B1"}, {ID: "b2", Title: "B2"},
	}}

	src := NewInterleavingSource(a, b)
	src.MaxPerSource = 3
	out, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	countA := 0
	for _, it := range out {
		if it.ID[0] == 'a' {
			countA++
		}
		if it.ID == "a4" || it.ID == "a5" {
			t.Errorf("item beyond cap present: %s", it.ID)
		}
	}
	if countA != 3 {
		t.Errorf("source A contributed %d items, want 3", countA)
	}
	if len(out) != 5 { // 3 from A + 2 from B
		t.Errorf("total %d, want 5: %+v", len(out), out)
	}
}

func TestInterleave_DedupsCrossPostTitles(t *testing.T) {
	a := fakeSource{name: "A", items: []core.Item{
		{ID: "a1", Title: "가수 청하", URL: "a1"},
		{ID: "a2", Title: "A second", URL: "a2"},
	}}
	b := fakeSource{name: "B", items: []core.Item{
		{ID: "b1", Title: "가수 청하", URL: "b1"}, // exact cross-post of a1
		{ID: "b2", Title: "B second", URL: "b2"},
	}}

	src := NewInterleavingSource(a, b)
	out, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error: %v", err)
	}

	// Round-robin order is a1,b1,a2,b2 -> b1 dropped as dup of a1.
	wantIDs := []string{"a1", "a2", "b2"}
	if len(out) != len(wantIDs) {
		t.Fatalf("got %d items, want %d: %+v", len(out), len(wantIDs), out)
	}
	for i, id := range wantIDs {
		if out[i].ID != id {
			t.Errorf("item %d = %q, want %q", i, out[i].ID, id)
		}
	}
}

func TestInterleave_AllSourcesFailedIsTotalFetchError(t *testing.T) {
	a := fakeSource{name: "A", err: errors.New("blocked")}
	b := fakeSource{name: "B", err: errors.New("layout changed")}

	src := NewInterleavingSource(a, b)
	out, err := src.Fetch(context.Background())
	if len(out) != 0 {
		t.Fatalf("got %d items, want 0", len(out))
	}
	var total *TotalFetchError
	if !errors.As(err, &total) {
		t.Fatalf("err = %v, want *TotalFetchError", err)
	}
	var partial *PartialFetchError
	if errors.As(err, &partial) {
		t.Errorf("total failure must not be a PartialFetchError")
	}
}

func TestInterleave_SomeSourcesFailedIsPartial(t *testing.T) {
	a := fakeSource{name: "A", items: []core.Item{{ID: "a1", Title: "A1"}}}
	b := fakeSource{name: "B", err: errors.New("blocked")}

	src := NewInterleavingSource(a, b)
	out, err := src.Fetch(context.Background())
	if len(out) != 1 {
		t.Fatalf("got %d items, want 1", len(out))
	}
	var partial *PartialFetchError
	if !errors.As(err, &partial) {
		t.Fatalf("err = %v, want *PartialFetchError", err)
	}
}

// budgetScraper records whether it was actually invoked.
type budgetScraper struct{ called bool }

func (s *budgetScraper) Name() string { return "budget" }
func (s *budgetScraper) FetchBestPosts(client *http.Client) ([]scraper.Post, error) {
	s.called = true
	return []scraper.Post{{Title: "t", URL: "https://example.com/1"}}, nil
}

// 예산(ctx) 소진 후의 Fetch 는 스크레이핑을 시작하지 않고 ctx 오류를 그대로
// 올려야 한다 — framework 의 예산 초과 경보 경로가 이 오류에 의존한다.
func TestAdapter_ExpiredBudgetStopsFetch(t *testing.T) {
	sc := &budgetScraper{}
	a := NewAdapter(sc)

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()

	items, err := a.Fetch(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
	if sc.called {
		t.Fatal("scraper must not run after the budget is spent")
	}
	if len(items) != 0 {
		t.Fatalf("want no items, got %d", len(items))
	}

	// 예산이 살아 있으면 정상 동작.
	items, err = a.Fetch(context.Background())
	if err != nil || len(items) != 1 || !sc.called {
		t.Fatalf("live budget fetch failed: items=%d err=%v called=%v", len(items), err, sc.called)
	}
}

// 감사 best-archive(B4): 루리웹 글 주소는 글 번호가 경로에 있고 쿼리는
// 「어느 목록에서 눌렀나」만 담는다. 그래서 같은 글이 목록 위치에 따라 다른
// 쿼리를 달고 나와 dedup 키가 갈라졌다 — 실측 414건 중 29건(7%)이 «?m=» 값만
// 달라 두 번 발송됐다. 2026-09-15 베스트 목록 실측에서 read 링크의 쿼리 키는
// m·t·cate·view 넷뿐이었고, 모두 글 신원과 무관하다.
func TestNormalizeURL_RuliwebDropsListContextQuery(t *testing.T) {
	const canonical = "https://bbs.ruliweb.com/best/board/300143/read/76678622"
	variants := []string{
		canonical,
		canonical + "?m=selection&t=now",
		canonical + "?m=user_info",
		canonical + "?cate=1035%2C1037%2C1039&view=gallery",
		canonical + "?m=selection&t=now#comment",
	}
	for _, in := range variants {
		if got := NormalizeURL(in); got != canonical {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, canonical)
		}
	}

	// 같은 글의 모든 형태가 하나의 dedup 키로 접혀야 한다.
	keys := map[string]bool{}
	for _, in := range variants {
		keys[NormalizeURL(in)] = true
	}
	if len(keys) != 1 {
		t.Errorf("dedup 키가 %d 갈래로 갈렸다: %v", len(keys), keys)
	}
}

// ⚠좁히기: MLB파크는 m=view 가 고정이고 글 번호도 쿼리에 있어서 쿼리를 비우면
// 글을 구별할 수 없게 된다 — 서로 다른 글이 한 키로 접혀 «두 번째 글이 유실»된다.
func TestNormalizeURL_MlbparkKeepsIdentifyingQuery(t *testing.T) {
	a := "https://mlbpark.donga.com/mp/b.php?m=view&b=bullpen&id=202509150001"
	b := "https://mlbpark.donga.com/mp/b.php?m=view&b=bullpen&id=202509150002"
	if got := NormalizeURL(a); got != a {
		t.Errorf("NormalizeURL(%q) = %q, 그대로여야 한다", a, got)
	}
	if NormalizeURL(a) == NormalizeURL(b) {
		t.Errorf("서로 다른 글이 한 키로 접혔다: %q", NormalizeURL(a))
	}
}

// 루리웹이라도 /read/ 가 아닌 목록·검색 주소는 건드리지 않는다.
func TestNormalizeURL_RuliwebNonReadPathUntouched(t *testing.T) {
	in := "https://bbs.ruliweb.com/search?q=%EA%B2%80%EC%83%89&board=300143"
	if got := NormalizeURL(in); got != in {
		t.Errorf("NormalizeURL(%q) = %q, 그대로여야 한다", in, got)
	}
}

// 호스트가 다르면 경로에 /read/ 가 있어도 건드리지 않는다.
func TestNormalizeURL_OtherHostReadPathUntouched(t *testing.T) {
	in := "https://example.com/board/read/123?m=selection"
	if got := NormalizeURL(in); got != in {
		t.Errorf("NormalizeURL(%q) = %q, 그대로여야 한다", in, got)
	}
}
