// Package scan walks the filesystem and feeds .md files into the scan pipeline.
package scan

import (
	"io/fs"
	"path/filepath"

	"github.com/olegmif/mdx/lsp/internal/config"
)

// Walk traverses root and calls fn for every regular file whose name ends
// in ".md". Directories whose base name appears in excludes are skipped
// entirely. Directories and files whose absolute path falls under any
// prefix in ignorePrefixes are also skipped (the walker does not descend
// into matching directories).
func Walk(root string, excludes []string, ignorePrefixes []string, fn func(path string, info fs.FileInfo) error) error {
	excludeSet := make(map[string]struct{}, len(excludes))
	for _, e := range excludes {
		excludeSet[e] = struct{}{}
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path == root {
				return nil
			}
			if _, skip := excludeSet[d.Name()]; skip {
				return fs.SkipDir
			}
			if config.IsIgnored(path, ignorePrefixes) {
				return fs.SkipDir
			}
			return nil
		}

		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		if config.IsIgnored(path, ignorePrefixes) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		return fn(path, info)
	})
}
