package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/olegmif/mdx/lsp/internal/db"
	"github.com/olegmif/mdx/lsp/internal/parse"
	"github.com/olegmif/mdx/lsp/internal/scan"
)

// ResolvedLink is one outgoing link with its target resolved to an absolute
// filesystem path. Callers use it to validate or render a link without
// re-parsing the file.
type ResolvedLink struct {
	RawTarget  string
	TargetPath string
	Line       int
	Col        int
}

// Result is what indexing produced for one file.
type Result struct {
	Path  string
	Links []ResolvedLink
}

// IndexFile reads path from disk and indexes its current contents.
func IndexFile(ctx context.Context, conn *sql.DB, path string) (Result, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read: %w", err)
	}
	return IndexBytes(ctx, conn, path, content)
}

// IndexBytes indexes content as the contents of path. inode/mtime/size are
// taken from path via Stat; the hash is computed from content (so an unsaved
// buffer can be indexed accurately).
func IndexBytes(ctx context.Context, conn *sql.DB, path string, content []byte) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	sum := sha256.Sum256(content)
	hash := hex.EncodeToString(sum[:])

	mtime, size, inode, err := scan.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("stat: %w", err)
	}

	fm, body, err := parse.Parse(content)
	if err != nil {
		return Result{}, fmt.Errorf("frontmatter: %w", err)
	}

	rawLinks := parse.ExtractLinks(body)
	tags := parse.ExtractTags(fm, body)

	sourceDir := filepath.Dir(path)
	resolved := make([]ResolvedLink, 0, len(rawLinks))
	linkRecords := make([]db.LinkRecord, 0, len(rawLinks))
	for _, l := range rawLinks {
		target := resolveTarget(sourceDir, l.RawTarget)
		resolved = append(resolved, ResolvedLink{
			RawTarget:  l.RawTarget,
			TargetPath: target,
			Line:       l.Line,
			Col:        l.Col,
		})
		linkRecords = append(linkRecords, db.LinkRecord{
			TargetPath: target,
			RawTarget:  l.RawTarget,
			Line:       l.Line,
			Col:        l.Col,
		})
	}

	fmJSON := ""
	if fm != nil {
		raw, err := json.Marshal(fm)
		if err != nil {
			return Result{}, fmt.Errorf("marshal frontmatter: %w", err)
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
		return Result{}, fmt.Errorf("begin: %w", err)
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
		return Result{}, err
	}
	if err := db.ReplaceLinks(tx, path, linkRecords); err != nil {
		return Result{}, err
	}
	if err := db.ReplaceTags(tx, path, tags); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit: %w", err)
	}

	return Result{Path: path, Links: resolved}, nil
}
