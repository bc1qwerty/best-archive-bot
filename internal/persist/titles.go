package persist

import (
	"database/sql"
	"time"
)

// Titles 는 «어느 커뮤니티가 이 제목을 먼저 보냈나» 를 실행 사이에 기억한다.
// 프레임워크 Store 와 같은 DB 파일에 둬서 GHA 캐시로 같이 넘어간다.
//
// ⚠ 한 실행 안의 제목 중복 제거(InterleavingSource)만으로는 부족하다. 거기서 밀린
//
//	글은 seen 처리가 안 돼서 다음 실행에 그대로 나갔고, 같은 글이 커뮤니티마다
//	몇 시간 차로 베스트에 오르는 경우는 애초에 못 잡았다. 2026-11-17 실측: 14일
//	3,615건 중 같은 제목이 다른 커뮤니티에서 다시 나간 경우가 대부분 2~37시간 차.
type Titles struct {
	db *sql.DB
}

func OpenTitles(db *sql.DB) (*Titles, error) {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS archive_title_seen (
		title_key TEXT PRIMARY KEY,
		community TEXT NOT NULL,
		seen_at INTEGER NOT NULL DEFAULT (unixepoch())
	)`)
	if err != nil {
		return nil, err
	}
	return &Titles{db: db}, nil
}

// SentElsewhere 는 within 안에 **다른** 커뮤니티가 같은 제목을 보냈는지 알려 준다.
// 같은 커뮤니티는 거르지 않는다 — 발송 실패로 미뤄진 글의 재시도가 자기 기록에
// 막히면 안 되고, 이토랜드 «아이브 장원영» 처럼 제목만 같은 별개 글도 있다.
func (t *Titles) SentElsewhere(key, community string, within time.Duration) (bool, error) {
	var cnt int
	err := t.db.QueryRow(
		`SELECT COUNT(*) FROM archive_title_seen WHERE title_key = ? AND community != ? AND seen_at >= ?`,
		key, community, time.Now().Add(-within).Unix()).Scan(&cnt)
	return cnt > 0, err
}

func (t *Titles) Record(key, community string) error {
	_, err := t.db.Exec(`
		INSERT INTO archive_title_seen (title_key, community) VALUES (?, ?)
		ON CONFLICT(title_key) DO UPDATE SET community = excluded.community, seen_at = unixepoch()`,
		key, community)
	return err
}

func (t *Titles) Cleanup(retain time.Duration) error {
	_, err := t.db.Exec(`DELETE FROM archive_title_seen WHERE seen_at < ?`, time.Now().Add(-retain).Unix())
	return err
}
