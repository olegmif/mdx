package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// Hash returns the lowercase hex SHA-256 of the file at path.
func Hash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("Hash open: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("Hash read: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
