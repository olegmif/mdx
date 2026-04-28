package cli

import (
	"context"
	"database/sql"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/db"
)

func TestScanEndToEnd(t *testing.T) {
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

	stats, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Errors != 0 {
		t.Errorf("errors = %d, want 0", stats.Errors)
	}
	if stats.Files != 5 {
		t.Errorf("files = %d, want 5", stats.Files)
	}

	if got := count(t, conn, "SELECT COUNT(*) FROM notes"); got != 5 {
		t.Errorf("notes = %d, want 5", got)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM links"); got != 3 {
		t.Errorf("links = %d, want 3", got)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM tags"); got != 4 {
		t.Errorf("tags = %d, want 4", got)
	}

	fmPath := filepath.Join(tmp, "with-frontmatter.md")
	var title string
	if err := conn.QueryRow(`SELECT title FROM notes WHERE path = ?`, fmPath).Scan(&title); err != nil {
		t.Fatalf("read title: %v", err)
	}
	if title != "With Frontmatter" {
		t.Errorf("title = %q, want %q", title, "With Frontmatter")
	}

	fmTags := selectTags(t, conn, fmPath)
	if want := []string{"intro", "sample"}; !reflect.DeepEqual(fmTags, want) {
		t.Errorf("frontmatter tags = %v, want %v", fmTags, want)
	}

	tagsPath := filepath.Join(tmp, "with-tags.md")
	bodyTags := selectTags(t, conn, tagsPath)
	if want := []string{"go", "notes/personal"}; !reflect.DeepEqual(bodyTags, want) {
		t.Errorf("body tags = %v, want %v", bodyTags, want)
	}

	linksPath := filepath.Join(tmp, "with-links.md")
	type linkRow struct{ target, raw string }
	rows, err := conn.Query(
		`SELECT target_path, raw_target FROM links WHERE source_path = ? ORDER BY raw_target`,
		linksPath,
	)
	if err != nil {
		t.Fatalf("query links: %v", err)
	}
	defer rows.Close()
	var got []linkRow
	for rows.Next() {
		var l linkRow
		if err := rows.Scan(&l.target, &l.raw); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, l)
	}
	want := []linkRow{
		{filepath.Join(tmp, "nested", "deep.md"), "./nested/deep.md"},
		{filepath.Join(tmp, "plain.md"), "./plain.md"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("links from with-links.md = %v, want %v", got, want)
	}

	deepPath := filepath.Join(tmp, "nested", "deep.md")
	var deepTarget, deepRaw string
	if err := conn.QueryRow(
		`SELECT target_path, raw_target FROM links WHERE source_path = ?`,
		deepPath,
	).Scan(&deepTarget, &deepRaw); err != nil {
		t.Fatalf("query deep link: %v", err)
	}
	if want := filepath.Join(tmp, "with-links.md"); deepTarget != want {
		t.Errorf("deep target = %q, want %q", deepTarget, want)
	}
	if deepRaw != "../with-links.md" {
		t.Errorf("deep raw = %q, want ../with-links.md", deepRaw)
	}

	// Re-running the scan must not duplicate rows (UPSERT behavior).
	if _, err := RunScan(context.Background(), conn, []string{tmp}, DefaultExcludes); err != nil {
		t.Fatalf("rerun Run: %v", err)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM notes"); got != 5 {
		t.Errorf("after rerun, notes = %d, want 5", got)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM links"); got != 3 {
		t.Errorf("after rerun, links = %d, want 3", got)
	}
	if got := count(t, conn, "SELECT COUNT(*) FROM tags"); got != 4 {
		t.Errorf("after rerun, tags = %d, want 4", got)
	}
}

func count(t *testing.T, conn *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := conn.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return n
}

func selectTags(t *testing.T, conn *sql.DB, path string) []string {
	t.Helper()
	rows, err := conn.Query(`SELECT tag FROM tags WHERE path = ? ORDER BY tag`, path)
	if err != nil {
		t.Fatalf("query tags: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, s)
	}
	return out
}

func copyFixturesToTmp(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatalf("copyFixturesToTmp: %v", err)
	}
	return dst
}
