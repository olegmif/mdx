// Package config loads user-level mdx configuration files
// (currently only the ignore list).
package config

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolveIgnorePath picks where the ignore file lives.
// Precedence: explicit override -> MDX_IGNORE env -> $XDG_CONFIG_HOME/mdx/ignore -> $HOME/.config/mdx/ignore.
// The path is returned even if the file does not exist; LoadIgnore handles that.
func ResolveIgnorePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("MDX_IGNORE"); env != "" {
		return env, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mdx", "ignore"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "mdx", "ignore"), nil
}

// LoadIgnore reads the ignore file at path and returns absolute path
// prefixes. Lines starting with '#' (after trimming whitespace) and
// blank lines are skipped. Leading '~' or '~/' is expanded against
// the current user's home directory. Lines that resolve to relative
// paths are skipped and reported via warnings, identified by file:line.
//
// If path is empty or the file does not exist, prefixes and warnings
// are nil and err is nil.
func LoadIgnore(path string) (prefixes []string, warnings []string, err error) {
	if path == "" {
		return nil, nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	home, homeErr := os.UserHomeDir()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}

		expanded := raw
		switch {
		case raw == "~":
			if homeErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d: cannot expand '~': %v", path, lineNo, homeErr))
				continue
			}
			expanded = home
		case strings.HasPrefix(raw, "~/"):
			if homeErr != nil {
				warnings = append(warnings, fmt.Sprintf("%s:%d: cannot expand '~/': %v", path, lineNo, homeErr))
				continue
			}
			expanded = filepath.Join(home, raw[2:])
		}

		if !filepath.IsAbs(expanded) {
			warnings = append(warnings, fmt.Sprintf("%s:%d: relative path %q ignored", path, lineNo, raw))
			continue
		}
		prefixes = append(prefixes, filepath.Clean(expanded))
	}
	if err := scanner.Err(); err != nil {
		return prefixes, warnings, fmt.Errorf("read %s: %w", path, err)
	}
	return prefixes, warnings, nil
}

// IsIgnored reports whether path falls under any prefix in prefixes.
// Match is exact equality or strict subtree (path begins with prefix
// followed by the OS path separator).
func IsIgnored(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return false
	}
	clean := filepath.Clean(path)
	sep := string(filepath.Separator)
	for _, p := range prefixes {
		if clean == p {
			return true
		}
		if strings.HasPrefix(clean, p+sep) {
			return true
		}
	}
	return false
}
