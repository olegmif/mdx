package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	content := []byte("hello world\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	mtime, size, inode, err := Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}
	now := time.Now().Unix()
	if mtime > now+5 || mtime < now-60 {
		t.Errorf("mtime = %d, want close to %d", mtime, now)
	}

	switch runtime.GOOS {
	case "linux", "darwin", "freebsd", "netbsd", "openbsd":
		if inode == 0 {
			t.Errorf("inode = 0, want non-zero on %s", runtime.GOOS)
		}
	}
}

func TestStatMissing(t *testing.T) {
	_, _, _, err := Stat("/nonexistent/__mdx_test_path__")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
