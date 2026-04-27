package scan

import (
	"fmt"
	"os"
)

// Stat returns mtime (unix secons), size (bytes), and indoe for the file
// at path. On platforms where inode is unavailable, inode is 0
func Stat(path string) (mtime, size, inode int64, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("stat: %w", err)
	}
	return fi.ModTime().Unix(), fi.Size(), inodeOf(fi), nil
}
