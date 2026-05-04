package db

import (
	"context"
	"database/sql"
	"fmt"
)

// PendingNote is one note that needs to be embedded for some model.
// Title и Frontmatter — nullable: их значение в notes может быть NULL,
// и нам важно различать «нет данных» от пустой строки при формировании
// payload в Qdrant.
type PendingNote struct {
	Path        string
	ContentHash string
	Title       sql.NullString
	Mtime       int64
	Frontmatter sql.NullString
}

// PendingEmbeddings returns notes that need embedding for the given model:
// either there is no row in the embeddings table, or the recorded
// content_hash no longer matches the current one in notes. If all=true
// every note is returned regardless of embeddings state.
func PendingEmbeddings(ctx context.Context, conn *sql.DB, model string, all bool) ([]PendingNote, error) {
	allFlag := 0
	if all {
		allFlag = 1
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT n.path, n.content_hash, n.title, n.mtime, n.frontmatter
		FROM notes n
		LEFT JOIN embeddings e
		  ON e.path = n.path AND e.model = ?
		WHERE ? = 1
		   OR e.path IS NULL
		   OR e.content_hash <> n.content_hash
	`, model, allFlag)
	if err != nil {
		return nil, fmt.Errorf("pending embeddings: %w", err)
	}
	defer rows.Close()

	var pending []PendingNote
	for rows.Next() {
		var p PendingNote
		if err := rows.Scan(&p.Path, &p.ContentHash, &p.Title, &p.Mtime, &p.Frontmatter); err != nil {
			return nil, fmt.Errorf("scan pending: %w", err)
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return pending, nil
}

// RecordEmbedding upserts a row into the embeddings table. On conflict
// over (path, model) content_hash и embedded_at обновляются.
func RecordEmbedding(ctx context.Context, conn *sql.DB, path, model, contentHash string, embeddedAt int64) error {
	_, err := conn.ExecContext(ctx, `
		INSERT INTO embeddings (path, model, content_hash, embedded_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path, model) DO UPDATE SET
		  content_hash = excluded.content_hash,
		  embedded_at  = excluded.embedded_at
	`, path, model, contentHash, embeddedAt)
	if err != nil {
		return fmt.Errorf("record embedding %s/%s: %w", path, model, err)
	}
	return nil
}
