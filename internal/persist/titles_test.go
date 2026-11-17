package persist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bc1qwerty/txid-bot-framework/pkg/store"
)

func TestTitles_OnlyOtherCommunitiesWithinWindow(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "posts.db"), "test")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	titles, err := OpenTitles(st.DB())
	if err != nil {
		t.Fatalf("OpenTitles: %v", err)
	}
	if err := titles.Record("같은제목", "이토랜드"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	check := func(key, community string, within time.Duration, want bool) {
		t.Helper()
		got, err := titles.SentElsewhere(key, community, within)
		if err != nil {
			t.Fatalf("SentElsewhere: %v", err)
		}
		if got != want {
			t.Errorf("SentElsewhere(%q, %q, %v) = %v, want %v", key, community, within, got, want)
		}
	}
	check("같은제목", "루리웹", time.Hour, true)
	check("같은제목", "이토랜드", time.Hour, false) // 같은 커뮤니티(재시도·별개 글)는 통과
	check("다른제목", "루리웹", time.Hour, false)

	// 기간이 지난 기록은 막지 않는다.
	if _, err := st.DB().Exec(`UPDATE archive_title_seen SET seen_at = seen_at - 7200`); err != nil {
		t.Fatalf("age: %v", err)
	}
	check("같은제목", "루리웹", time.Hour, false)

	if err := titles.Cleanup(time.Hour); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM archive_title_seen`).Scan(&n); err != nil || n != 0 {
		t.Errorf("after cleanup rows=%d err=%v, want 0", n, err)
	}
}
