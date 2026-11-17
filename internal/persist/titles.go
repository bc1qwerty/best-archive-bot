package persist

import (
	"database/sql"
	"strings"
	"time"
)

// Titles 는 «어느 커뮤니티의 어느 글이 이 제목을 먼저 보냈나» 를 실행 사이에 기억한다.
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
		seen_at INTEGER NOT NULL DEFAULT (unixepoch()),
		item_id TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return nil, err
	}
	// 첫 판(2026-11-17 da55ef0)은 item_id 없이 표를 만들었다. sqlite 에는
	// ADD COLUMN IF NOT EXISTS 가 없어서 이미 있다는 오류만 넘긴다.
	if _, err := db.Exec(`ALTER TABLE archive_title_seen ADD COLUMN item_id TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return nil, err
	}
	return &Titles{db: db}, nil
}

// Duplicate 는 within 안에 같은 제목이 이미 나갔는지 알려 준다. 다른 커뮤니티가
// 보낸 것이면 항상 중복이고, 같은 커뮤니티가 보낸 것은 sameCommunity 일 때만
// «다른 글» 에 한해 중복으로 친다(뽐뿌 특가 글 재등록).
//
// ⚠ 같은 글(itemID 일치)은 거르면 안 된다 — 발송 실패로 미뤄진 글의 재시도가
//
//	자기 기록에 막힌다. item_id 가 빈 옛 행도 같은 이유로 같은 커뮤니티에서는
//	중복으로 치지 않는다.
func (t *Titles) Duplicate(key, community, itemID string, sameCommunity bool, within time.Duration) (bool, error) {
	var cnt int
	err := t.db.QueryRow(`
		SELECT COUNT(*) FROM archive_title_seen
		WHERE title_key = ? AND seen_at >= ?
		  AND (community != ? OR (? AND item_id != '' AND item_id != ?))`,
		key, time.Now().Add(-within).Unix(), community, sameCommunity, itemID).Scan(&cnt)
	return cnt > 0, err
}

func (t *Titles) Record(key, community, itemID string) error {
	_, err := t.db.Exec(`
		INSERT INTO archive_title_seen (title_key, community, item_id) VALUES (?, ?, ?)
		ON CONFLICT(title_key) DO UPDATE SET
			community = excluded.community, item_id = excluded.item_id, seen_at = unixepoch()`,
		key, community, itemID)
	return err
}

func (t *Titles) Cleanup(retain time.Duration) error {
	_, err := t.db.Exec(`DELETE FROM archive_title_seen WHERE seen_at < ?`, time.Now().Add(-retain).Unix())
	return err
}
