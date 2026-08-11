package persist

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/bc1qwerty/txid-bot-framework/pkg/store"
)

// copyMainDBOnly copies just the ".db" file, mimicking the GHA cache which
// snapshots `data/posts.db` but not its `-wal` sidecar.
func copyMainDBOnly(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create dst: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("copy: %v", err)
	}
}

// TestCheckpointPersistsAcrossMainDBOnlyCopy reproduces the cache bug and
// proves Checkpoint fixes it: without a checkpoint, copying only the main
// .db file loses bot_seen; with a checkpoint, it survives.
func TestCheckpointPersistsAcrossMainDBOnlyCopy(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "posts.db")

	st, err := store.Open(dbPath, "test")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.MarkSeen("src", "id1"); err != nil {
		t.Fatalf("mark seen: %v", err)
	}

	// Bug reproduction: main .db copied without -wal loses the write.
	buggy := filepath.Join(dir, "buggy.db")
	copyMainDBOnly(t, dbPath, buggy)
	stBug, err := store.Open(buggy, "test")
	if err != nil {
		t.Fatalf("open buggy: %v", err)
	}
	seen, err := stBug.IsSeen("src", "id1")
	if err != nil {
		t.Fatalf("is seen buggy: %v", err)
	}
	_ = stBug.Close()
	if seen {
		t.Skip("environment auto-checkpointed; bug not reproducible here")
	}

	// Fix: checkpoint folds the WAL into the main .db before the snapshot.
	if err := Checkpoint(st.DB()); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
	fixed := filepath.Join(dir, "fixed.db")
	copyMainDBOnly(t, dbPath, fixed)
	_ = st.Close()

	stFixed, err := store.Open(fixed, "test")
	if err != nil {
		t.Fatalf("open fixed: %v", err)
	}
	defer stFixed.Close()
	seen, err = stFixed.IsSeen("src", "id1")
	if err != nil {
		t.Fatalf("is seen fixed: %v", err)
	}
	if !seen {
		t.Fatal("after checkpoint, bot_seen did not survive main-db-only copy")
	}
}
