package db

import (
	"database/sql"
	"fmt"
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
