package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/db"
)

func TestGCRemovesDeletedFile(t *testing.T) {
	tmp := copyFixturesToTmp(t, "testdata/fixtures")

	dbPath := filepath.Join(t.TempDir(), "mdx.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes, nil); err != nil {
		t.Fatalf("RunScan: %v", err)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM notes"); got != 5 {
		t.Fatalf("after scan, notes = %d, want 5", got)
	}

	// with-links.md has two outgoing links — useful to confirm cascade.
	linksPath := filepath.Join(tmp, "with-links.md")
	if got := count(t, conn,
		"SELECT COUNT(*) FROM links WHERE source_path = '"+linksPath+"'"); got != 2 {
		t.Fatalf("links from with-links.md = %d, want 2 (sanity)", got)
	}

	if err := os.Remove(linksPath); err != nil {
		t.Fatalf("remove: %v", err)
	}

	stats, err := RunGC(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.Deleted != 1 {
		t.Errorf("deleted = %d, want 1", stats.Deleted)
	}
	if stats.Kept != 4 {
		t.Errorf("kept = %d, want 4", stats.Kept)
	}

	if got := count(t, conn, "SELECT COUNT(*) FROM notes"); got != 4 {
		t.Errorf("notes after gc = %d, want 4", got)
	}
	// FK CASCADE: outgoing links from the deleted note must be gone.
	if got := count(t, conn,
		"SELECT COUNT(*) FROM links WHERE source_path = '"+linksPath+"'"); got != 0 {
		t.Errorf("links from deleted note = %d, want 0 (cascade)", got)
	}
	// Links pointing TO the deleted note (incoming) stay — they represent
	// broken links that the user still has in surviving notes.
	if got := count(t, conn,
		"SELECT COUNT(*) FROM links WHERE target_path = '"+linksPath+"'"); got != 1 {
		t.Errorf("incoming links to deleted note = %d, want 1 (preserved)", got)
	}
}

func TestGCRemovesUnderIgnorePrefix(t *testing.T) {
	tmp := copyFixturesToTmp(t, "testdata/fixtures")

	dbPath := filepath.Join(t.TempDir(), "mdx.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes, nil); err != nil {
		t.Fatalf("RunScan: %v", err)
	}

	nestedPrefix := filepath.Join(tmp, "nested")
	stats, err := RunGC(context.Background(), conn, []string{nestedPrefix})
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.Deleted != 1 {
		t.Errorf("deleted = %d, want 1 (nested/deep.md)", stats.Deleted)
	}
	if stats.Kept != 4 {
		t.Errorf("kept = %d, want 4", stats.Kept)
	}

	deepPath := filepath.Join(tmp, "nested", "deep.md")
	if got := count(t, conn,
		"SELECT COUNT(*) FROM notes WHERE path = '"+deepPath+"'"); got != 0 {
		t.Errorf("ignored note still in DB")
	}
}

func TestGCNoOpOnCleanDB(t *testing.T) {
	tmp := copyFixturesToTmp(t, "testdata/fixtures")

	dbPath := filepath.Join(t.TempDir(), "mdx.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes, nil); err != nil {
		t.Fatalf("RunScan: %v", err)
	}

	stats, err := RunGC(context.Background(), conn, nil)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.Deleted != 0 {
		t.Errorf("deleted = %d, want 0", stats.Deleted)
	}
	if stats.Kept != 5 {
		t.Errorf("kept = %d, want 5", stats.Kept)
	}
}

func TestGCKeepsExistingFilesNotIgnored(t *testing.T) {
	// gc has no concept of "scan roots" — files that exist on disk and
	// are not under an ignore prefix must be kept regardless of where
	// they live.
	tmp := copyFixturesToTmp(t, "testdata/fixtures")

	dbPath := filepath.Join(t.TempDir(), "mdx.db")
	conn, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes, nil); err != nil {
		t.Fatalf("RunScan: %v", err)
	}

	// Ignore prefix that does not match any indexed file.
	stats, err := RunGC(context.Background(), conn, []string{"/nonexistent/prefix"})
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.Deleted != 0 {
		t.Errorf("deleted = %d, want 0", stats.Deleted)
	}
	if stats.Kept != 5 {
		t.Errorf("kept = %d, want 5", stats.Kept)
	}
}
