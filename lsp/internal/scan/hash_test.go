package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.md")
	content := []byte("hello world\n")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Hash(path)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:])
	if got != want {
		t.Errorf("hash = %s, want %s", got, want)
	}
}

func TestHashMissing(t *testing.T) {
	_, err := Hash("/nonexistent/__mdx_test_path__")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}
