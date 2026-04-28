package lsp

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

func Init(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir log: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	level := slog.LevelInfo
	if os.Getenv("MDX_LOG") == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})))
	return nil
}

func ResolveLogPath(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "mdx", "lsp.log"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve $HOME: %w", err)
	}
	return filepath.Join(home, ".local", "state", "mdx", "lsp.log"), nil
}
