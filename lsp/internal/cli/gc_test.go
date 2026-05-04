package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
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

	stats, err := RunGC(context.Background(), conn, nil, nil)
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
	stats, err := RunGC(context.Background(), conn, []string{nestedPrefix}, nil)
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

	stats, err := RunGC(context.Background(), conn, nil, nil)
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
	stats, err := RunGC(context.Background(), conn, []string{"/nonexistent/prefix"}, nil)
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

func TestGCQdrantHappyPath(t *testing.T) {
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

	// Снести один файл — в mdx-фазе уйдёт его строка, в Qdrant-фазе должна
	// уйти и его точка (если бы была — но мы её в моке не возвращаем,
	// так как сценарий «уже отсутствует в Qdrant» здесь не проверяется).
	if err := os.Remove(filepath.Join(tmp, "with-links.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	survivor := filepath.Join(tmp, "plain.md")

	var deleted []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/collections/mdx/points/scroll":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
				"result": {
					"points": [
						{"id":"id-keep","payload":{"path":%q}},
						{"id":"id-orphan-1","payload":{"path":"/nonexistent/orphan-1.md"}},
						{"id":"id-orphan-2","payload":{"path":"/nonexistent/orphan-2.md"}}
					],
					"next_page_offset": null
				},
				"status": "ok"
			}`, survivor)
		case "/collections/mdx/points/delete":
			var body struct {
				Points []string `json:"points"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode delete: %v", err)
			}
			deleted = append(deleted, body.Points...)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"result":{},"status":"ok"}`)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	cfg := &config.EmbeddingConfig{QdrantURL: srv.URL, Collection: "mdx"}

	stats, err := RunGC(context.Background(), conn, nil, cfg)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}

	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if stats.Kept != 4 {
		t.Errorf("Kept = %d, want 4", stats.Kept)
	}
	if stats.QdrantSkipped {
		t.Error("QdrantSkipped = true, want false")
	}
	if stats.QdrantFailed {
		t.Error("QdrantFailed = true, want false")
	}
	if stats.QdrantDeleted != 2 {
		t.Errorf("QdrantDeleted = %d, want 2", stats.QdrantDeleted)
	}
	if stats.QdrantKept != 1 {
		t.Errorf("QdrantKept = %d, want 1", stats.QdrantKept)
	}

	want := []string{"id-orphan-1", "id-orphan-2"}
	if len(deleted) != len(want) || deleted[0] != want[0] || deleted[1] != want[1] {
		t.Errorf("delete request ids = %v, want %v", deleted, want)
	}
}

func TestGCQdrantUnavailable(t *testing.T) {
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
	if err := os.Remove(filepath.Join(tmp, "with-links.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	// Закрытый сервер: URL валиден, но коннект завершается отказом.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	cfg := &config.EmbeddingConfig{QdrantURL: srv.URL, Collection: "mdx"}

	stats, err := RunGC(context.Background(), conn, nil, cfg)
	if err != nil {
		t.Fatalf("RunGC: want nil error, got %v", err)
	}
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1 (mdx-фаза должна была отработать)", stats.Deleted)
	}
	if stats.Kept != 4 {
		t.Errorf("Kept = %d, want 4", stats.Kept)
	}
	if stats.QdrantSkipped {
		t.Error("QdrantSkipped = true, want false (фаза запускалась)")
	}
	if !stats.QdrantFailed {
		t.Error("QdrantFailed = false, want true")
	}
	if stats.QdrantDeleted != 0 {
		t.Errorf("QdrantDeleted = %d, want 0 (scroll упал)", stats.QdrantDeleted)
	}
}

func TestGCQdrantSkippedWithoutConfig(t *testing.T) {
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
	if err := os.Remove(filepath.Join(tmp, "with-links.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	stats, err := RunGC(context.Background(), conn, nil, nil)
	if err != nil {
		t.Fatalf("RunGC: %v", err)
	}
	if stats.Deleted != 1 {
		t.Errorf("Deleted = %d, want 1", stats.Deleted)
	}
	if !stats.QdrantSkipped {
		t.Error("QdrantSkipped = false, want true")
	}
	if stats.QdrantFailed {
		t.Error("QdrantFailed = true, want false")
	}
	if stats.QdrantDeleted != 0 {
		t.Errorf("QdrantDeleted = %d, want 0", stats.QdrantDeleted)
	}
}
