package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func setupDB(t *testing.T) *sql.DB {
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

func mustCommit(t *testing.T, tx *sql.Tx) {
	t.Helper()
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestUpsertNote(t *testing.T) {
	conn := setupDB(t)

	note := NoteRecord{
		Path:        "/notes/a.md",
		Inode:       42,
		Mtime:       1700000000,
		Size:        1024,
		ContentHash: "deadbeef",
		Frontmatter: `{"title":"A"}`,
		Title:       "A",
	}

	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := UpsertNote(tx, note); err != nil {
		t.Fatalf("UpsertNote: %v", err)
	}
	mustCommit(t, tx)

	var got NoteRecord
	err = conn.QueryRow(
		`SELECT path, inode, mtime, size, content_hash, frontmatter, title                                                                                                                                   
                 FROM notes WHERE path = ?`,
		note.Path,
	).Scan(&got.Path, &got.Inode, &got.Mtime, &got.Size,
		&got.ContentHash, &got.Frontmatter, &got.Title)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != note {
		t.Errorf("got %#v, want %#v", got, note)
	}

	// Re-upsert with new fields → row count stays 1, fields update.
	note.ContentHash = "newhash"
	note.Size = 2048

	tx, _ = conn.Begin()
	if err := UpsertNote(tx, note); err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}
	mustCommit(t, tx)

	var count int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM notes WHERE path = ?`, note.Path,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("rows = %d, want 1", count)
	}

	var hash string
	conn.QueryRow(
		`SELECT content_hash FROM notes WHERE path = ?`, note.Path,
	).Scan(&hash)
	if hash != "newhash" {
		t.Errorf("hash = %q, want newhash", hash)
	}
}

func TestReplaceLinks(t *testing.T) {
	conn := setupDB(t)

	// Insert parent note (FK requirement).
	tx, _ := conn.Begin()
	if err := UpsertNote(tx, NoteRecord{
		Path: "/n.md", Mtime: 1, Size: 1, ContentHash: "h",
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	mustCommit(t, tx)

	tx, _ = conn.Begin()
	if err := ReplaceLinks(tx, "/n.md", []LinkRecord{
		{TargetPath: "/x.md", RawTarget: "x.md", Line: 1, Col: 1},
		{TargetPath: "/y.md", RawTarget: "./y.md", Line: 2, Col: 5},
	}); err != nil {
		t.Fatalf("ReplaceLinks: %v", err)
	}
	mustCommit(t, tx)

	var count int
	conn.QueryRow(
		`SELECT COUNT(*) FROM links WHERE source_path = ?`, "/n.md",
	).Scan(&count)
	if count != 2 {
		t.Errorf("after first insert, links = %d, want 2", count)
	}

	// Replace with a single new link.
	tx, _ = conn.Begin()
	if err := ReplaceLinks(tx, "/n.md", []LinkRecord{
		{TargetPath: "/z.md", RawTarget: "z.md", Line: 3, Col: 1},
	}); err != nil {
		t.Fatalf("ReplaceLinks 2: %v", err)
	}
	mustCommit(t, tx)

	conn.QueryRow(
		`SELECT COUNT(*) FROM links WHERE source_path = ?`, "/n.md",
	).Scan(&count)
	if count != 1 {
		t.Errorf("after replace, links = %d, want 1", count)
	}

	var target string
	conn.QueryRow(
		`SELECT target_path FROM links WHERE source_path = ?`, "/n.md",
	).Scan(&target)
	if target != "/z.md" {
		t.Errorf("target = %q, want /z.md", target)
	}
}

func TestReplaceTags(t *testing.T) {
	conn := setupDB(t)

	tx, _ := conn.Begin()
	if err := UpsertNote(tx, NoteRecord{
		Path: "/n.md", Mtime: 1, Size: 1, ContentHash: "h",
	}); err != nil {
		t.Fatalf("seed note: %v", err)
	}
	mustCommit(t, tx)

	tx, _ = conn.Begin()
	if err := ReplaceTags(tx, "/n.md", []string{"go", "rust"}); err != nil {
		t.Fatalf("ReplaceTags: %v", err)
	}
	mustCommit(t, tx)

	var count int
	conn.QueryRow(
		`SELECT COUNT(*) FROM tags WHERE path = ?`, "/n.md",
	).Scan(&count)
	if count != 2 {
		t.Errorf("after first insert, tags = %d, want 2", count)
	}

	// Replace with a different set.
	tx, _ = conn.Begin()
	if err := ReplaceTags(tx, "/n.md", []string{"python"}); err != nil {
		t.Fatalf("ReplaceTags 2: %v", err)
	}
	mustCommit(t, tx)

	conn.QueryRow(
		`SELECT COUNT(*) FROM tags WHERE path = ?`, "/n.md",
	).Scan(&count)
	if count != 1 {
		t.Errorf("after replace, tags = %d, want 1", count)
	}

	var tag string
	conn.QueryRow(
		`SELECT tag FROM tags WHERE path = ?`, "/n.md",
	).Scan(&tag)
	if tag != "python" {
		t.Errorf("tag = %q, want python", tag)
	}
}

func TestListNotesEmpty(t *testing.T) {
	conn := setupDB(t)

	got, err := ListNotes(conn)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

func TestListNotesSorted(t *testing.T) {
	conn := setupDB(t)

	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	seed := []NoteRecord{
		{Path: "/notes/c.md", Title: "Charlie"},
		{Path: "/notes/a.md", Title: "alpha"},
		{Path: "/notes/b.md", Title: ""},
		{Path: "/notes/d.md", Title: "Bravo"},
	}
	for _, n := range seed {
		if err := UpsertNote(tx, n); err != nil {
			t.Fatalf("UpsertNote: %v", err)
		}
	}
	mustCommit(t, tx)

	got, err := ListNotes(conn)
	if err != nil {
		t.Fatalf("ListNotes: %v", err)
	}

	want := []NoteEntry{
		{Path: "/notes/a.md", Title: "alpha"},
		{Path: "/notes/d.md", Title: "Bravo"},
		{Path: "/notes/c.md", Title: "Charlie"},
		{Path: "/notes/b.md", Title: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
