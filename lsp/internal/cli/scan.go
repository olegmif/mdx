// Package cli implements the high-level commands invoked from cmd/mdx.
package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/olegmif/mdx/lsp/internal/db"
	"github.com/olegmif/mdx/lsp/internal/parse"
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
			if err := scanOne(conn, path); err != nil {
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

func scanOne(conn *sql.DB, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	hash, err := scan.Hash(path)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	mtime, size, inode, err := scan.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	fm, body, err := parse.Parse(content)
	if err != nil {
		return fmt.Errorf("frontmatter: %w", err)
	}

	rawLinks := parse.ExtractLinks(body)
	tags := parse.ExtractTags(fm, body)

	sourceDir := filepath.Dir(path)
	linkRecords := make([]db.LinkRecord, 0, len(rawLinks))
	for _, l := range rawLinks {
		linkRecords = append(linkRecords, db.LinkRecord{
			TargetPath: resolveTarget(sourceDir, l.RawTarget),
			RawTarget:  l.RawTarget,
			Line:       l.Line,
			Col:        l.Col,
		})
	}

	fmJSON := ""
	if fm != nil {
		raw, err := json.Marshal(fm)
		if err != nil {
			return fmt.Errorf("marshal frontmatter: %w", err)
		}
		fmJSON = string(raw)
	}

	title := ""
	if fm != nil {
		if t, ok := fm["title"].(string); ok {
			title = t
		}
	}

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if err := db.UpsertNote(tx, db.NoteRecord{
		Path:        path,
		Inode:       inode,
		Mtime:       mtime,
		Size:        size,
		ContentHash: hash,
		Frontmatter: fmJSON,
		Title:       title,
	}); err != nil {
		return err
	}
	if err := db.ReplaceLinks(tx, path, linkRecords); err != nil {
		return err
	}
	if err := db.ReplaceTags(tx, path, tags); err != nil {
		return err
	}
	return tx.Commit()
}

// resolveTarget converts a link target as written in markdown to an absolute
// filesystem path. It expands a leading "~" and resolves relative paths
// against sourceDir.
func resolveTarget(sourceDir, raw string) string {
	if strings.HasPrefix(raw, "~/") || raw == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(raw, "~"))
		}
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Join(sourceDir, raw)
}
