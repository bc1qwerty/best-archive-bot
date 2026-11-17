package persist

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bc1qwerty/txid-bot-framework/pkg/store"
)

func TestTitles_Duplicate(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "posts.db"), "test")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	// 첫 판이 만든 item_id 없는 표에서 올라오는 경로.
	if _, err := st.DB().Exec(`CREATE TABLE archive_title_seen (
		title_key TEXT PRIMARY KEY, community TEXT NOT NULL,
		seen_at INTEGER NOT NULL DEFAULT (unixepoch()))`); err != nil {
		t.Fatalf("old schema: %v", err)
	}
	if _, err := st.DB().Exec(`INSERT INTO archive_title_seen (title_key, community) VALUES ('옛행', '뽐뿌')`); err != nil {
		t.Fatalf("old row: %v", err)
	}
	titles, err := OpenTitles(st.DB())
	if err != nil {
		t.Fatalf("OpenTitles: %v", err)
	}
	if _, err := OpenTitles(st.DB()); err != nil { // 두 번째 실행도 통과해야 한다
		t.Fatalf("OpenTitles again: %v", err)
	}
	if err := titles.Record("같은제목", "뽐뿌", "p1"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	check := func(key, community, id string, same bool, within time.Duration, want bool) {
		t.Helper()
		got, err := titles.Duplicate(key, community, id, same, within)
		if err != nil {
			t.Fatalf("Duplicate: %v", err)
		}
		if got != want {
			t.Errorf("Duplicate(%q, %q, %q, %v, %v) = %v, want %v", key, community, id, same, within, got, want)
		}
	}
	check("같은제목", "루리웹", "r1", false, time.Hour, true)
	check("같은제목", "뽐뿌", "p2", true, time.Hour, true)   // 재등록
	check("같은제목", "뽐뿌", "p1", true, time.Hour, false)  // 같은 글의 재시도
	check("같은제목", "뽐뿌", "p2", false, time.Hour, false) // 같은 커뮤니티 검사를 끈 곳
	check("옛행", "뽐뿌", "p9", true, time.Hour, false)    // 글 번호 모르는 옛 행
	check("옛행", "루리웹", "r9", true, time.Hour, true)
	check("다른제목", "루리웹", "r1", true, time.Hour, false)

	// 기간이 지난 기록은 막지 않는다.
	if _, err := st.DB().Exec(`UPDATE archive_title_seen SET seen_at = seen_at - 7200`); err != nil {
		t.Fatalf("age: %v", err)
	}
	check("같은제목", "루리웹", "r1", true, time.Hour, false)

	if err := titles.Cleanup(time.Hour); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	var n int
	if err := st.DB().QueryRow(`SELECT COUNT(*) FROM archive_title_seen`).Scan(&n); err != nil || n != 0 {
		t.Errorf("after cleanup rows=%d err=%v, want 0", n, err)
	}
}
