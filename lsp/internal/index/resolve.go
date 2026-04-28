package index

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveTarget converts a link target as written in markdown to an absolute
// filesystem path. Leading "~" is expanded; relative paths are resolved
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
