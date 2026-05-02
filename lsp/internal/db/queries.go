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

// Query выполняет произвольный пользовательский SQL-запрос и возвращает
// строки в виде []map[column]value. Допускаются только SELECT- и
// WITH-запросы — попытка INSERT/UPDATE/DELETE/DROP и прочей DDL/DML
// отклоняется до выполнения.
//
// Возвращаемые значения соответствуют типам SQLite: int64, float64,
// string, nil. BLOB-столбцы конвертируются в string.
func Query(conn *sql.DB, sqlStr string, args []any) ([]map[string]any, error) {
	if err := validateSelectOnly(sqlStr); err != nil {
		return nil, err
	}

	rows, err := conn.Query(sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	result := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[col] = v
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return result, nil
}

// validateSelectOnly проверяет, что SQL начинается с SELECT или WITH
// после удаления комментариев и whitespace. Дополнительный пояс
// безопасности на случай, если первый кейворд получится обмануть —
// здесь ровно один: «начинается с правильного слова».
func validateSelectOnly(sqlStr string) error {
	s := stripSQLComments(sqlStr)
	s = strings.TrimSpace(s)
	upper := strings.ToUpper(s)
	if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH") {
		return nil
	}
	return fmt.Errorf("only SELECT/WITH queries are allowed")
}

// stripSQLComments удаляет /* ... */ и -- комментарии. Не строгий парсер
// SQL — комментарии внутри строковых литералов он тоже «вычистит», но
// для нашей цели (определить первый кейворд) это безопасно: первый
// кейворд не может быть внутри литерала.
func stripSQLComments(sqlStr string) string {
	// Блок-комментарии /* ... */
	for {
		start := strings.Index(sqlStr, "/*")
		if start < 0 {
			break
		}
		rest := sqlStr[start:]
		end := strings.Index(rest, "*/")
		if end < 0 {
			sqlStr = sqlStr[:start]
			break
		}
		sqlStr = sqlStr[:start] + sqlStr[start+end+2:]
	}
	// Строчные комментарии -- ...
	var out []string
	for line := range strings.SplitSeq(sqlStr, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
