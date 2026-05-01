package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// NoteRecord is one row of the notes table.
type NoteRecord struct {
	Path        string
	Inode       int64
	Mtime       int64
	Size        int64
	ContentHash string
	Frontmatter string // serialized JSON; empty string when no frontmatter
	Title       string
}

// LinkRecord is one row of the links table (source_path is supplied separately).
type LinkRecord struct {
	TargetPath string
	RawTarget  string
	Line       int
	Col        int
}

type NoteEntry struct {
	Path  string `json:"path"`
	Title string `json:"title"`
}

// UpsertNote inserts or replaces a row in the notes table by path.
func UpsertNote(tx *sql.Tx, n NoteRecord) error {
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO notes 
			(path, inode, mtime, size, content_hash, frontmatter, title) 
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.Path, n.Inode, n.Mtime, n.Size, n.ContentHash, n.Frontmatter, n.Title,
	)
	if err != nil {
		return fmt.Errorf("upsert note %s: %w", n.Path, err)
	}
	return nil
}

// ReplaceLinks deletes all links from sourcePath and inserts the new set.
func ReplaceLinks(tx *sql.Tx, sourcePath string, links []LinkRecord) error {
	if _, err := tx.Exec(
		`DELETE FROM links WHERE source_path = ?`, sourcePath,
	); err != nil {
		return fmt.Errorf("delete links for %s: %w", sourcePath, err)
	}
	for _, l := range links {
		if _, err := tx.Exec(
			`INSERT INTO links (source_path, target_path, raw_target, line, col)
			  VALUES (?, ?, ?, ?, ?)`,
			sourcePath, l.TargetPath, l.RawTarget, l.Line, l.Col,
		); err != nil {
			return fmt.Errorf("insert link to %s: %w", l.TargetPath, err)
		}
	}
	return nil
}

// ReplaceTags deletes all tags for sourcePath and inserts the new set.
func ReplaceTags(tx *sql.Tx, sourcePath string, tags []string) error {
	if _, err := tx.Exec(
		`DELETE FROM tags WHERE path = ?`, sourcePath,
	); err != nil {
		return fmt.Errorf("delete tags for %s: %w", sourcePath, err)
	}
	for _, t := range tags {
		if _, err := tx.Exec(
			`INSERT INTO tags (path, tag) VALUES (?, ?)`,
			sourcePath, t,
		); err != nil {
			return fmt.Errorf("insert tag %q: %w", t, err)
		}
	}
	return nil
}

func ListNotes(db *sql.DB) ([]NoteEntry, error) {
	rows, err := db.Query(`
		SELECT path, COALESCE(title, '') AS title
		FROM notes
		ORDER BY (COALESCE(title, '') = '') ASC,
		         title COLLATE NOCASE,
		         path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []NoteEntry{}
	for rows.Next() {
		var entry NoteEntry
		if err := rows.Scan(&entry.Path, &entry.Title); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

func SearchByTags(db *sql.DB, include []string, exclude []string) ([]NoteEntry, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT path, COALESCE(title, '') AS title FROM notes`)

	var conds []string
	var args []any

	for _, t := range include {
		cond, arg := tagCondition(t)
		conds = append(conds, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM tags WHERE path = notes.path AND %s)", cond))
		args = append(args, arg)
	}
	for _, t := range exclude {
		cond, arg := tagCondition(t)
		conds = append(conds, fmt.Sprintf(
			"NOT EXISTS (SELECT 1 FROM tags WHERE path = notes.path AND %s)", cond))
		args = append(args, arg)
	}

	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}
	sb.WriteString(` ORDER BY (COALESCE(title, '') = '') ASC, title COLLATE NOCASE, path`)
	rows, err := db.Query(sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []NoteEntry{}
	for rows.Next() {
		var entry NoteEntry
		if err := rows.Scan(&entry.Path, &entry.Title); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// tagCondition returns the SQL fragment and argument for a single token.
// Парсер тегов M0 (parse.tagInBody, regex `[\w][\w/-]*`) исключает
// `*`, `?`, `[`, `]` из реальных имён тегов — поэтому появление любого
// из них в токене однозначно означает wildcard, и эскейп не нужен.
func tagCondition(token string) (sqlFragment string, arg string) {
	if strings.ContainsAny(token, "*?[") {
		return "tag GLOB ?", token
	}
	return "tag = ?", token
}
