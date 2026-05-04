package db

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestMigrateFreshCreatesTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mdx.db")

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	for _, table := range []string{"schema_version", "notes", "links", "tags", "embeddings"} {
		var name string
		err := conn.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}

	versions := readSchemaVersions(t, conn)
	if !reflect.DeepEqual(versions, []int{currentVersion}) {
		t.Errorf("schema_version rows = %v, want [%d]", versions, currentVersion)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mdx.db")

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	if err := Migrate(conn); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}

	before := readSchemaVersions(t, conn)

	if err := Migrate(conn); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}

	after := readSchemaVersions(t, conn)
	if !reflect.DeepEqual(before, after) {
		t.Errorf("schema_version after re-migrate: before=%v, after=%v", before, after)
	}
}

func TestMigrateFromV1(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mdx.db")

	conn, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	// Воссоздаём состояние v1: только schema_version, notes, links, tags
	// (то, что лежало в schema.sql до Шага 3) с записью version=1.
	const v1Schema = `
CREATE TABLE schema_version (
  version INTEGER PRIMARY KEY
);

CREATE TABLE notes (
  path          TEXT PRIMARY KEY,
  inode         INTEGER,
  mtime         INTEGER NOT NULL,
  size          INTEGER NOT NULL,
  content_hash  TEXT NOT NULL,
  frontmatter   TEXT,
  title         TEXT
);

CREATE INDEX idx_notes_inode        ON notes(inode);
CREATE INDEX idx_notes_content_hash ON notes(content_hash);

CREATE TABLE links (
  source_path  TEXT NOT NULL,
  target_path  TEXT NOT NULL,
  raw_target   TEXT NOT NULL,
  line         INTEGER NOT NULL,
  col          INTEGER NOT NULL,
  FOREIGN KEY (source_path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_links_source ON links(source_path);
CREATE INDEX idx_links_target ON links(target_path);

CREATE TABLE tags (
  path  TEXT NOT NULL,
  tag   TEXT NOT NULL,
  PRIMARY KEY (path, tag),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);

CREATE INDEX idx_tags_tag ON tags(tag);

INSERT INTO schema_version (version) VALUES (1);
`
	if _, err := conn.Exec(v1Schema); err != nil {
		t.Fatalf("seed v1 schema: %v", err)
	}

	// Кладём строку в notes — после миграции она должна остаться нетронутой.
	if _, err := conn.Exec(
		`INSERT INTO notes (path, mtime, size, content_hash) VALUES (?, ?, ?, ?)`,
		"/tmp/note.md", int64(1), int64(0), "deadbeef",
	); err != nil {
		t.Fatalf("seed notes row: %v", err)
	}

	if err := Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// embeddings создана.
	var name string
	if err := conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='embeddings'`,
	).Scan(&name); err != nil {
		t.Fatalf("embeddings missing after migrate: %v", err)
	}

	// Существующая строка в notes не пострадала.
	var hash string
	if err := conn.QueryRow(
		`SELECT content_hash FROM notes WHERE path = ?`, "/tmp/note.md",
	).Scan(&hash); err != nil {
		t.Fatalf("notes row lost: %v", err)
	}
	if hash != "deadbeef" {
		t.Errorf("content_hash = %q, want deadbeef", hash)
	}

	// schema_version содержит обе строки: 1 (исходная) и 2 (после миграции).
	versions := readSchemaVersions(t, conn)
	if !reflect.DeepEqual(versions, []int{1, 2}) {
		t.Errorf("schema_version rows = %v, want [1 2]", versions)
	}
}

func readSchemaVersions(t *testing.T, conn *sql.DB) []int {
	t.Helper()
	rows, err := conn.Query(`SELECT version FROM schema_version`)
	if err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	defer rows.Close()
	var versions []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("scan: %v", err)
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	sort.Ints(versions)
	return versions
}
