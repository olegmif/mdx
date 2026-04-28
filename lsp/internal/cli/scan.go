// Package cli implements the high-level commands invoked from cmd/mdx.
package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/olegmif/mdx/lsp/internal/index"
	"github.com/olegmif/mdx/lsp/internal/scan"
)

// Stats summarizes one scan run.
type Stats struct {
	Files   int           // .md files successfully processed
	Errors  int           // per-file errors (file skipped)
	Elapsed time.Duration // total wall time
}

// DefaultExcludes is the hardcoded list of directory base names that the
// scanner refuses to descend into.
var DefaultExcludes = []string{
	".git", "node_modules", ".venv", "target",
	".cache", "dist", "build", "vendor",
}

// Run scans every root, parsing each .md file and persisting metadata,
// outgoing links, and tags into the database. Per-file errors are reported
// to stderr and counted; they do not abort the run.
func Run(ctx context.Context, conn *sql.DB, roots, excludes []string) (Stats, error) {
	start := time.Now()
	var stats Stats

	for _, root := range roots {
		err := scan.Walk(root, excludes, func(path string, info fs.FileInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if _, err := index.IndexFile(ctx, conn, path); err != nil {
				stats.Errors++
				fmt.Fprintf(os.Stderr, "mdx: %s: %v\n", path, err)
				return nil
			}
			stats.Files++
			return nil
		})
		if err != nil {
			stats.Elapsed = time.Since(start)
			return stats, fmt.Errorf("walk %s: %w", root, err)
		}
	}

	stats.Elapsed = time.Since(start)
	return stats, nil
}
