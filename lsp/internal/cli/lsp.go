package cli

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/olegmif/mdx/lsp/internal/lsp"
)

func RunLSP(ctx context.Context, conn *sql.DB, dbPath, logPath string, ignore []string) error {
	if err := lsp.Init(logPath); err != nil {
		return err
	}

	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	slog.Info("server starting",
		"version", version, "db", dbPath, "log", logPath, "ignore_count", len(ignore))

	go func() {
		<-ctx.Done()
		slog.Info("server stopped", "reason", ctx.Err())
		if err := conn.Close(); err != nil {
			slog.Error("db close", "err", err)
		}
		os.Exit(0)
	}()

	srv := lsp.New(conn, ignore)
	return srv.RunStdio()
}
