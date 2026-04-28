package cli

import (
	"context"
	"database/sql"
	"log/slog"
	"runtime/debug"

	"github.com/olegmif/mdx/lsp/internal/lsp"
)

func RunLSP(ctx context.Context, conn *sql.DB, dbPath, logPath string) error {
	if err := lsp.Init(logPath); err != nil {
		return err
	}

	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	slog.Info("server starting", "version", version, "db", dbPath, "log", logPath)

	srv := lsp.New(conn)
	return srv.RunStdio()
}
