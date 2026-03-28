package db

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/bc1qwerty/best-archive-bot/internal/config"
	"github.com/bc1qwerty/best-archive-bot/internal/scraper"
)

// KST is the Korea Standard Time zone.
var KST = time.FixedZone("KST", 9*60*60)

// PostDB manages the sent_posts database.
type PostDB struct {
	dbPath string
	db     *sql.DB
}

// New creates a new PostDB instance.
func New() *PostDB {
	return &PostDB{dbPath: config.DBPath}
}

// Init creates the database and table if they don't exist.
func (pdb *PostDB) Init() error {
	// Ensure parent directory exists
	dir := filepath.Dir(pdb.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", pdb.dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	pdb.db = db

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sent_posts (
			url_hash TEXT PRIMARY KEY,
			url TEXT NOT NULL,
			community TEXT NOT NULL,
			sent_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_sent_at ON sent_posts(sent_at)")
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	log.Println("DB 초기화 완료")
	return nil
}

// Close closes the database connection.
func (pdb *PostDB) Close() error {
	if pdb.db != nil {
		return pdb.db.Close()
	}
	return nil
}

// hashURL normalizes a URL and returns its SHA256 hash.
func hashURL(rawURL string) string {
	// Remove fragment
	if idx := strings.Index(rawURL, "#"); idx != -1 {
		rawURL = rawURL[:idx]
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		// Fall back to raw hash
		h := sha256.Sum256([]byte(rawURL))
		return fmt.Sprintf("%x", h)
	}

	// Normalize: always https
	scheme := "https"
	// Lowercase host, strip trailing slash
	host := strings.ToLower(parsed.Hostname())
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/"
	}

	// Sort query parameters
	params := parsed.Query()
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queryParts []string
	for _, k := range keys {
		for _, v := range params[k] {
			queryParts = append(queryParts, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	query := strings.Join(queryParts, "&")

	var normalized string
	if query != "" {
		normalized = fmt.Sprintf("%s://%s%s?%s", scheme, host, path, query)
	} else {
		normalized = fmt.Sprintf("%s://%s%s", scheme, host, path)
	}

	h := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("%x", h)
}

// FilterUnsent returns only posts that haven't been sent yet.
func (pdb *PostDB) FilterUnsent(posts []scraper.Post) ([]scraper.Post, error) {
	if len(posts) == 0 {
		return nil, nil
	}

	// Build hash-to-post map
	hashMap := make(map[string]scraper.Post, len(posts))
	hashes := make([]string, 0, len(posts))
	for _, p := range posts {
		h := hashURL(p.URL)
		if _, exists := hashMap[h]; !exists {
			hashMap[h] = p
			hashes = append(hashes, h)
		}
	}

	// Batch query: check which hashes exist
	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, h := range hashes {
		placeholders[i] = "?"
		args[i] = h
	}

	query := fmt.Sprintf(
		"SELECT url_hash FROM sent_posts WHERE url_hash IN (%s)",
		strings.Join(placeholders, ","),
	)
	rows, err := pdb.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("filter query: %w", err)
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("scan hash: %w", err)
		}
		existing[h] = true
	}

	var unsent []scraper.Post
	for _, h := range hashes {
		if !existing[h] {
			unsent = append(unsent, hashMap[h])
		}
	}
	return unsent, nil
}

// MarkSent records posts as sent in the database.
func (pdb *PostDB) MarkSent(posts []scraper.Post) error {
	if len(posts) == 0 {
		return nil
	}

	now := time.Now().In(KST).Format(time.RFC3339)
	tx, err := pdb.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		"INSERT OR IGNORE INTO sent_posts (url_hash, url, community, sent_at) VALUES (?, ?, ?, ?)",
	)
	if err != nil {
		return fmt.Errorf("prepare: %w", err)
	}
	defer stmt.Close()

	for _, p := range posts {
		h := hashURL(p.URL)
		if _, err := stmt.Exec(h, p.URL, p.Community, now); err != nil {
			return fmt.Errorf("exec mark_sent: %w", err)
		}
	}

	return tx.Commit()
}

// CleanupOldRecords deletes records older than RecordExpireHours.
func (pdb *PostDB) CleanupOldRecords() error {
	cutoff := time.Now().In(KST).Add(-time.Duration(config.RecordExpireHours) * time.Hour).Format(time.RFC3339)
	result, err := pdb.db.Exec("DELETE FROM sent_posts WHERE sent_at < ?", cutoff)
	if err != nil {
		return fmt.Errorf("cleanup: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		log.Printf("만료 레코드 %d건 삭제", affected)
	}
	return nil
}
