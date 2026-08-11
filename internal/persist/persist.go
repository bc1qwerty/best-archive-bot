// Package persist holds helpers that make the framework SQLite store
// survive the one-shot GHA run model.
package persist

import "database/sql"

// Checkpoint folds the write-ahead log back into the main database file and
// truncates the WAL. The framework opens the store in WAL mode, so without
// this the bot_seen/bot_sent rows written during a run live only in the
// "<db>-wal" sidecar file. The GHA cache snapshots the main ".db" file, so
// an un-checkpointed WAL means every run restores an empty dedup table and
// re-dispatches the current top posts. Call this before the process exits.
func Checkpoint(db *sql.DB) error {
	_, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}
