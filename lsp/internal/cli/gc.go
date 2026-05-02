package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/olegmif/mdx/lsp/internal/config"
)

// GCStats summarizes one gc run.
type GCStats struct {
	Deleted int           // notes rows removed
	Kept    int           // notes rows that survived
	Elapsed time.Duration // total wall time
}

// RunGC removes from the database every notes row whose file is missing
// from disk or whose path falls under any prefix in ignorePrefixes. FK
// ON DELETE CASCADE drops dependent rows in links and tags.
//
// Stat errors other than fs.ErrNotExist (typically permission issues)
// are reported to stderr and the row is kept — better to leave a row we
// could not verify than to silently drop it.
func RunGC(ctx context.Context, conn *sql.DB, ignorePrefixes []string) (GCStats, error) {
	start := time.Now()
	var stats GCStats

	rows, err := conn.QueryContext(ctx, `SELECT path FROM notes`)
	if err != nil {
		stats.Elapsed = time.Since(start)
		return stats, fmt.Errorf("select notes: %w", err)
	}
	var dbPaths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			stats.Elapsed = time.Since(start)
			return stats, fmt.Errorf("scan path: %w", err)
		}
		dbPaths = append(dbPaths, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		stats.Elapsed = time.Since(start)
		return stats, fmt.Errorf("rows: %w", err)
	}

	var orphans []string
	for _, p := range dbPaths {
		if err := ctx.Err(); err != nil {
			stats.Elapsed = time.Since(start)
			return stats, err
		}
		if config.IsIgnored(p, ignorePrefixes) {
			orphans = append(orphans, p)
			continue
		}
		if _, err := os.Lstat(p); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				orphans = append(orphans, p)
				continue
			}
			fmt.Fprintf(os.Stderr, "mdx: gc: stat %s: %v\n", p, err)
		}
	}

	if len(orphans) > 0 {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			stats.Elapsed = time.Since(start)
			return stats, fmt.Errorf("begin tx: %w", err)
		}
		stmt, err := tx.PrepareContext(ctx, `DELETE FROM notes WHERE path = ?`)
		if err != nil {
			tx.Rollback()
			stats.Elapsed = time.Since(start)
			return stats, fmt.Errorf("prepare delete: %w", err)
		}
		for _, p := range orphans {
			if _, err := stmt.ExecContext(ctx, p); err != nil {
				stmt.Close()
				tx.Rollback()
				stats.Elapsed = time.Since(start)
				return stats, fmt.Errorf("delete %s: %w", p, err)
			}
			stats.Deleted++
		}
		stmt.Close()
		if err := tx.Commit(); err != nil {
			stats.Elapsed = time.Since(start)
			return stats, fmt.Errorf("commit: %w", err)
		}
	}

	stats.Kept = len(dbPaths) - stats.Deleted
	stats.Elapsed = time.Since(start)
	return stats, nil
}
