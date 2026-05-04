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
	"github.com/olegmif/mdx/lsp/internal/embed"
)

// GCStats summarizes one gc run.
type GCStats struct {
	Deleted       int           // notes rows removed
	Kept          int           // notes rows that survived
	QdrantDeleted int           // qdrant points removed
	QdrantKept    int           // qdrant points kept
	QdrantSkipped bool          // qdrant phase did not run (no embedding config)
	QdrantFailed  bool          // qdrant phase ran and produced at least one error
	Elapsed       time.Duration // total wall time
}

// RunGC removes from the database every notes row whose file is missing
// from disk or whose path falls under any prefix in ignorePrefixes. FK
// ON DELETE CASCADE drops dependent rows in links, tags and embeddings.
//
// Stat errors other than fs.ErrNotExist (typically permission issues)
// are reported to stderr and the row is kept — better to leave a row we
// could not verify than to silently drop it.
//
// embedCfg, when non-nil, triggers a follow-up Qdrant cleanup phase that
// removes points whose payload.path is no longer in notes. Any failure
// of the Qdrant phase is logged to stderr and reflected in
// stats.QdrantFailed; it does not propagate as an error from RunGC.
// embedCfg == nil skips the Qdrant phase entirely (stats.QdrantSkipped).
// The full Qdrant phase is wired in Steps 4–5 of M3_embeddings; for now
// only the skip branch is honoured.
func RunGC(ctx context.Context, conn *sql.DB, ignorePrefixes []string, embedCfg *config.EmbeddingConfig) (GCStats, error) {
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

	if embedCfg == nil {
		stats.QdrantSkipped = true
	} else {
		notesPaths, err := loadNotesPaths(ctx, conn)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdx: gc: qdrant: read notes paths: %v\n", err)
			stats.QdrantFailed = true
		} else {
			deleted, kept, failed := cleanQdrant(ctx, *embedCfg, notesPaths)
			stats.QdrantDeleted = deleted
			stats.QdrantKept = kept
			stats.QdrantFailed = stats.QdrantFailed || failed
		}
	}

	stats.Elapsed = time.Since(start)
	return stats, nil
}

// loadNotesPaths reads the current set of paths from notes into a
// map. Used by the Qdrant cleanup phase to compute the diff against
// scrolled point payloads.
func loadNotesPaths(ctx context.Context, conn *sql.DB) (map[string]struct{}, error) {
	rows, err := conn.QueryContext(ctx, `SELECT path FROM notes`)
	if err != nil {
		return nil, fmt.Errorf("select notes: %w", err)
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan path: %w", err)
		}
		out[p] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// cleanQdrant performs the Qdrant phase of gc: scrolls every point in
// the collection, treats as orphan any point whose payload.path is not
// in notesPaths (or is missing entirely), and deletes those points.
//
// The function never returns an error: any failure (network, HTTP
// non-2xx, parse) is logged to stderr as `mdx: gc: qdrant: <op>: <err>`
// and reported via failed=true. deleted/kept reflect counts up to the
// last successful step — on a delete failure deleted stays 0 (the
// orphan batch could not be confirmed removed), kept reflects the
// scroll outcome.
func cleanQdrant(ctx context.Context, cfg config.EmbeddingConfig, notesPaths map[string]struct{}) (deleted, kept int, failed bool) {
	qd := embed.NewQdrantClient(cfg.QdrantURL)

	points, err := qd.Scroll(ctx, cfg.Collection, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mdx: gc: qdrant: scroll: %v\n", err)
		return 0, 0, true
	}

	var orphans []string
	for _, p := range points {
		if p.Path == "" {
			orphans = append(orphans, p.ID)
			continue
		}
		if _, ok := notesPaths[p.Path]; !ok {
			orphans = append(orphans, p.ID)
			continue
		}
		kept++
	}

	if len(orphans) == 0 {
		return 0, kept, false
	}

	if err := qd.DeletePoints(ctx, cfg.Collection, orphans); err != nil {
		fmt.Fprintf(os.Stderr, "mdx: gc: qdrant: delete %d points: %v\n", len(orphans), err)
		return 0, kept, true
	}
	return len(orphans), kept, false
}
