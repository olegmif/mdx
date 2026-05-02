package db

import (
	"database/sql"
	"path/filepath"
	"reflect"
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

func seed(t *testing.T, conn *sql.DB, notes []NoteRecord, tagsByPath map[string][]string) {
	t.Helper()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		if err := UpsertNote(tx, n); err != nil {
			t.Fatalf("UpsertNote: %v", err)
		}
	}
	for path, tags := range tagsByPath {
		if err := ReplaceTags(tx, path, tags); err != nil {
			t.Fatalf("ReplaceTags: %v", err)
		}
	}
	mustCommit(t, tx)
}

func entryPaths(entries []NoteEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Path
	}
	return out
}

func TestSearchByTagsEmptyDB(t *testing.T) {
	conn := setupDB(t)

	got, err := SearchByTags(conn, nil, nil)
	if err != nil {
		t.Fatalf("SearchByTags: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want empty slice")
	}
	if len(got) != 0 {
		t.Errorf("got %d entries, want 0", len(got))
	}
}

func TestSearchByTags(t *testing.T) {
	conn := setupDB(t)
	seed(t, conn,
		[]NoteRecord{
			{Path: "/n/a.md", Title: "alpha"},
			{Path: "/n/b.md", Title: "Bravo"},
			{Path: "/n/c.md", Title: "Charlie"},
			{Path: "/n/d.md", Title: "delta"},
			{Path: "/n/e.md", Title: "echo"},
			{Path: "/n/f.md", Title: "foxtrot"},
			{Path: "/n/tg.md", Title: "Tag/g"},
			{Path: "/n/t1.md", Title: "tag1"},
			{Path: "/n/t2.md", Title: "tag2"},
		},
		map[string][]string{
			"/n/a.md":  {"go", "mdx"},
			"/n/b.md":  {"mdx", "mdx-design"},
			"/n/c.md":  {"go", "draft"},
			"/n/d.md":  {"mdx-impl", "mdx-draft"},
			"/n/e.md":  {"vim"},
			"/n/tg.md": {"tag", "tog", "tug"},
			"/n/t1.md": {"tag1"},
			"/n/t2.md": {"tag2"},
			// /n/f.md — без тегов
		},
	)

	allTagged := []string{
		"/n/a.md", "/n/b.md", "/n/c.md", "/n/d.md", "/n/e.md",
		"/n/tg.md", "/n/t1.md", "/n/t2.md",
	}
	allNotes := []string{
		"/n/a.md", "/n/b.md", "/n/c.md", "/n/d.md", "/n/e.md",
		"/n/f.md", "/n/tg.md", "/n/t1.md", "/n/t2.md",
	}

	tests := []struct {
		name    string
		include []string
		exclude []string
		want    []string
	}{
		{
			name: "empty filters → all notes, sorted by title",
			want: allNotes,
		},
		{
			name:    "single exact include",
			include: []string{"mdx"},
			want:    []string{"/n/a.md", "/n/b.md"},
		},
		{
			name:    "AND of two exact includes",
			include: []string{"go", "mdx"},
			want:    []string{"/n/a.md"},
		},
		{
			name:    "only exclude",
			exclude: []string{"draft"},
			want: []string{
				"/n/a.md", "/n/b.md", "/n/d.md", "/n/e.md",
				"/n/f.md", "/n/tg.md", "/n/t1.md", "/n/t2.md",
			},
		},
		{
			name:    "include + exclude",
			include: []string{"mdx"},
			exclude: []string{"draft"},
			want:    []string{"/n/a.md", "/n/b.md"},
		},
		{
			name:    "include nonexistent → empty",
			include: []string{"nonexistent"},
			want:    []string{},
		},
		{
			name:    "exclude nonexistent → no narrowing",
			exclude: []string{"nonexistent"},
			want:    allNotes,
		},
		{
			name:    "prefix wildcard",
			include: []string{"mdx*"},
			want:    []string{"/n/a.md", "/n/b.md", "/n/d.md"},
		},
		{
			name:    "suffix wildcard",
			include: []string{"*draft"},
			want:    []string{"/n/c.md", "/n/d.md"},
		},
		{
			name:    "substring wildcard exclude",
			exclude: []string{"*draft*"},
			want: []string{
				"/n/a.md", "/n/b.md", "/n/e.md", "/n/f.md",
				"/n/tg.md", "/n/t1.md", "/n/t2.md",
			},
		},
		{
			name:    "exact + wildcard mix",
			include: []string{"go", "mdx*"},
			want:    []string{"/n/a.md"},
		},
		{
			name:    "lone star include → tagged only",
			include: []string{"*"},
			want:    allTagged,
		},
		{
			name:    "lone star exclude → untagged only",
			exclude: []string{"*"},
			want:    []string{"/n/f.md"},
		},
		{
			name:    "wildcard include + exact exclude",
			include: []string{"mdx*"},
			exclude: []string{"mdx-draft"},
			want:    []string{"/n/a.md", "/n/b.md"},
		},
		{
			name:    "single-char wildcard",
			include: []string{"t?g"},
			want:    []string{"/n/tg.md"},
		},
		{
			name:    "char class",
			include: []string{"tag[12]"},
			want:    []string{"/n/t1.md", "/n/t2.md"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SearchByTags(conn, tc.include, tc.exclude)
			if err != nil {
				t.Fatalf("SearchByTags: %v", err)
			}
			gotPaths := entryPaths(got)
			if !reflect.DeepEqual(gotPaths, tc.want) {
				t.Errorf("got  %v\nwant %v", gotPaths, tc.want)
			}
		})
	}
}

func TestQueryValidationRejectsNonSelect(t *testing.T) {
	conn := setupDB(t)

	cases := []string{
		"INSERT INTO notes(path) VALUES('x')",
		"DELETE FROM notes",
		"UPDATE notes SET title='x'",
		"DROP TABLE notes",
		"CREATE TABLE x(y INT)",
		"PRAGMA query_only = OFF",
		"   ",
		"",
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			_, err := Query(conn, q, nil)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", q)
			}
		})
	}
}

func TestQueryValidationAcceptsSelectAndWith(t *testing.T) {
	conn := setupDB(t)

	cases := []string{
		"SELECT 1",
		"  select 1",
		"-- comment\nSELECT 1",
		"/* block */ SELECT 1",
		"WITH x AS (SELECT 1) SELECT * FROM x",
	}
	for _, q := range cases {
		t.Run(q, func(t *testing.T) {
			_, err := Query(conn, q, nil)
			if err != nil {
				t.Fatalf("expected ok for %q, got %v", q, err)
			}
		})
	}
}

func TestQueryReturnsRowsAsMaps(t *testing.T) {
	conn := setupDB(t)
	seed(t, conn,
		[]NoteRecord{
			{Path: "/a.md", Title: "Alpha", Frontmatter: `{"type":"task","priority":1}`},
			{Path: "/b.md", Title: "Bravo", Frontmatter: `{"type":"note"}`},
			{Path: "/c.md", Title: "Charlie", Frontmatter: `{"type":"task"}`},
		},
		nil,
	)

	got, err := Query(conn,
		`SELECT path, COALESCE(title, '') AS title
		 FROM notes
		 WHERE json_extract(frontmatter, '$.type') = ?
		 ORDER BY path`,
		[]any{"task"},
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(got), got)
	}
	if got[0]["path"] != "/a.md" || got[0]["title"] != "Alpha" {
		t.Errorf("row 0 = %+v", got[0])
	}
	if got[1]["path"] != "/c.md" || got[1]["title"] != "Charlie" {
		t.Errorf("row 1 = %+v", got[1])
	}
}

func TestQueryEmptyResultIsEmptySlice(t *testing.T) {
	conn := setupDB(t)
	got, err := Query(conn, "SELECT path FROM notes WHERE path = ?", []any{"/nope"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want []")
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0", len(got))
	}
}

func TestStripSQLComments(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"SELECT 1", "SELECT 1"},
		{"-- hi\nSELECT 1", "\nSELECT 1"},
		{"SELECT 1 -- trailing", "SELECT 1 "},
		{"/* block */ SELECT 1", " SELECT 1"},
		{"SELECT /* mid */ 1", "SELECT  1"},
		{"/* unclosed", ""},
		{"/* a */ /* b */ SELECT 1", "  SELECT 1"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := stripSQLComments(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTagCondition(t *testing.T) {
	tests := []struct {
		token   string
		wantSQL string
	}{
		{"mdx", "tag = ?"},
		{"mdx-design", "tag = ?"},
		{"mdx*", "tag GLOB ?"},
		{"*tmp", "tag GLOB ?"},
		{"*draft*", "tag GLOB ?"},
		{"t?g", "tag GLOB ?"},
		{"tag[12]", "tag GLOB ?"},
		{"*", "tag GLOB ?"},
	}
	for _, tc := range tests {
		t.Run(tc.token, func(t *testing.T) {
			gotSQL, gotArg := tagCondition(tc.token)
			if gotSQL != tc.wantSQL {
				t.Errorf("sqlFragment = %q, want %q", gotSQL, tc.wantSQL)
			}
			if gotArg != tc.token {
				t.Errorf("arg = %q, want %q (token passed through unchanged)", gotArg, tc.token)
			}
		})
	}
}
