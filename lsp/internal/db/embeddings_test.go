package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"
)

func openMigrated(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := Open(filepath.Join(t.TempDir(), "mdx.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

// seedNote creates a notes row with the given content_hash; other fields
// are filled with deterministic placeholders.
func seedNote(t *testing.T, conn *sql.DB, path, contentHash string) {
	t.Helper()
	_, err := conn.Exec(
		`INSERT INTO notes (path, mtime, size, content_hash, title, frontmatter) VALUES (?, ?, ?, ?, ?, ?)`,
		path, int64(1), int64(0), contentHash, "T:"+path, sql.NullString{},
	)
	if err != nil {
		t.Fatalf("seed note %s: %v", path, err)
	}
}

func pendingPaths(t *testing.T, conn *sql.DB, model string, all bool) []string {
	t.Helper()
	pending, err := PendingEmbeddings(context.Background(), conn, model, all)
	if err != nil {
		t.Fatalf("PendingEmbeddings: %v", err)
	}
	paths := make([]string, 0, len(pending))
	for _, p := range pending {
		paths = append(paths, p.Path)
	}
	sort.Strings(paths)
	return paths
}

func TestPendingEmbeddingsLifecycle(t *testing.T) {
	conn := openMigrated(t)
	ctx := context.Background()

	seedNote(t, conn, "/n/a.md", "ha")
	seedNote(t, conn, "/n/b.md", "hb")

	// Empty embeddings: both notes are pending.
	if got, want := pendingPaths(t, conn, "m1", false), []string{"/n/a.md", "/n/b.md"}; !equalStrings(got, want) {
		t.Errorf("after seed: got %v, want %v", got, want)
	}

	// Record one — only the other stays pending.
	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha", 100); err != nil {
		t.Fatalf("RecordEmbedding: %v", err)
	}
	if got, want := pendingPaths(t, conn, "m1", false), []string{"/n/b.md"}; !equalStrings(got, want) {
		t.Errorf("after record: got %v, want %v", got, want)
	}

	// Update content_hash on the recorded note — it becomes pending again.
	if _, err := conn.Exec(`UPDATE notes SET content_hash = ? WHERE path = ?`, "ha2", "/n/a.md"); err != nil {
		t.Fatalf("update notes: %v", err)
	}
	if got, want := pendingPaths(t, conn, "m1", false), []string{"/n/a.md", "/n/b.md"}; !equalStrings(got, want) {
		t.Errorf("after content_hash bump: got %v, want %v", got, want)
	}

	// all=true: both pending regardless of embeddings state. Re-record to
	// align hashes first, чтобы доказать, что фильтрация по content_hash
	// действительно отключается.
	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha2", 200); err != nil {
		t.Fatalf("RecordEmbedding: %v", err)
	}
	if err := RecordEmbedding(ctx, conn, "/n/b.md", "m1", "hb", 200); err != nil {
		t.Fatalf("RecordEmbedding: %v", err)
	}
	if got := pendingPaths(t, conn, "m1", false); len(got) != 0 {
		t.Errorf("after full record, all=false: got %v, want empty", got)
	}
	if got, want := pendingPaths(t, conn, "m1", true), []string{"/n/a.md", "/n/b.md"}; !equalStrings(got, want) {
		t.Errorf("all=true: got %v, want %v", got, want)
	}
}

func TestRecordEmbeddingUpsert(t *testing.T) {
	conn := openMigrated(t)
	ctx := context.Background()
	seedNote(t, conn, "/n/a.md", "ha")

	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha", 100); err != nil {
		t.Fatalf("first record: %v", err)
	}
	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha2", 200); err != nil {
		t.Fatalf("second record: %v", err)
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE path = ? AND model = ?`,
		"/n/a.md", "m1").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("embeddings rows = %d, want 1 (upsert)", count)
	}

	var hash string
	var embeddedAt int64
	if err := conn.QueryRow(
		`SELECT content_hash, embedded_at FROM embeddings WHERE path = ? AND model = ?`,
		"/n/a.md", "m1",
	).Scan(&hash, &embeddedAt); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if hash != "ha2" || embeddedAt != 200 {
		t.Errorf("row = (%s, %d), want (ha2, 200)", hash, embeddedAt)
	}
}

func TestEmbeddingsCascadeOnNoteDelete(t *testing.T) {
	conn := openMigrated(t)
	ctx := context.Background()
	seedNote(t, conn, "/n/a.md", "ha")
	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha", 100); err != nil {
		t.Fatalf("RecordEmbedding: %v", err)
	}

	if _, err := conn.Exec(`DELETE FROM notes WHERE path = ?`, "/n/a.md"); err != nil {
		t.Fatalf("delete note: %v", err)
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM embeddings WHERE path = ?`, "/n/a.md").Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("embeddings rows after note delete = %d, want 0", count)
	}
}

func TestPendingEmbeddingsModelIsolation(t *testing.T) {
	conn := openMigrated(t)
	ctx := context.Background()
	seedNote(t, conn, "/n/a.md", "ha")

	// Запись для m1 не должна влиять на pending для m2.
	if err := RecordEmbedding(ctx, conn, "/n/a.md", "m1", "ha", 100); err != nil {
		t.Fatalf("RecordEmbedding: %v", err)
	}
	if got := pendingPaths(t, conn, "m1", false); len(got) != 0 {
		t.Errorf("m1 pending = %v, want empty", got)
	}
	if got, want := pendingPaths(t, conn, "m2", false), []string{"/n/a.md"}; !equalStrings(got, want) {
		t.Errorf("m2 pending = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
