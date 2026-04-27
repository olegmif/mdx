// Package db provide SQLite connection setup, schema migrations and queries for mdx metadata.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens (creating if missing) the SQLite database at path
// and applies required pragmas.
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA temp_store=MEMORY",
	}
	for _, p := range pragmas {
		if _, err := conn.Exec(p); err != nil {
			conn.Close()
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}

	return conn, nil
}

// ResolvePath picks where the database file lives.
// Precedence: explicit override -> MDX_DB env -> $XDG_DATA_HOME/mdx/mdx.db -> $HOME/.local/share/mdx/mdx.db.
func ResolvePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("MDX_DB"); env != "" {
		return env, nil
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "mdx", "mdx.db"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "share", "mdx", "mdx.db"), nil
}
