package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestWalk(t *testing.T) {
	root := t.TempDir()

	// Build a small tree under root:
	//   note1.md, note2.txt
	//   sub/note3.md, sub/img.png
	//   .git/HEAD, .git/notes.md       (excluded dir)
	//   node_modules/foo.md            (excluded dir)
	//   nested/deeper/note4.md
	files := map[string]string{
		"note1.md":               "# 1",
		"note2.txt":              "not markdown",
		"sub/note3.md":           "# 3",
		"sub/img.png":            "png bytes",
		".git/HEAD":              "head",
		".git/notes.md":          "# inside excluded",
		"node_modules/foo.md":    "# inside excluded",
		"nested/deeper/note4.md": "# 4",
	}

	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}

	excludes := []string{".git", "node_modules"}

	var visited []string
	err := Walk(root, excludes, nil, func(path string, info fs.FileInfo) error {
		rel, _ := filepath.Rel(root, path)
		visited = append(visited, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	sort.Strings(visited)
	want := []string{
		"nested/deeper/note4.md",
		"note1.md",
		"sub/note3.md",
	}
	if !reflect.DeepEqual(visited, want) {
		t.Errorf("visited = %v, want %v", visited, want)
	}
}

func TestWalkIgnorePrefixes(t *testing.T) {
	root := t.TempDir()

	files := map[string]string{
		"keep.md":               "# keep",
		"sub/keep.md":            "# keep",
		"state/log.md":           "# ignored subtree",
		"state/inner/deep.md":    "# ignored subtree",
		"stateful/keep.md":       "# string-prefix collision, must be visited",
		"other/exact.md":         "# matched exactly by ignore prefix path",
	}

	for rel, content := range files {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	ignorePrefixes := []string{
		filepath.Join(root, "state"),
		filepath.Join(root, "other", "exact.md"),
	}

	var visited []string
	err := Walk(root, nil, ignorePrefixes, func(path string, info fs.FileInfo) error {
		rel, _ := filepath.Rel(root, path)
		visited = append(visited, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}

	sort.Strings(visited)
	want := []string{
		"keep.md",
		"stateful/keep.md",
		"sub/keep.md",
	}
	if !reflect.DeepEqual(visited, want) {
		t.Errorf("visited = %v, want %v", visited, want)
	}
}
