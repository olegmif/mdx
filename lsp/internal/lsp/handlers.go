package lsp

import (
	"log/slog"
	"os"
	"runtime/debug"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func ptr[T any](v T) *T { return &v }

func (s *Server) onInitialize(ctx *glsp.Context, params *protocol.InitializeParams) (any, error) {
	syncKind := protocol.TextDocumentSyncKindNone
	capabilities := protocol.ServerCapabilities{
		TextDocumentSync: protocol.TextDocumentSyncOptions{
			OpenClose: ptr(true),
			Change:    &syncKind,
			Save:      protocol.SaveOptions{IncludeText: ptr(false)},
		},
	}

	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}

	slog.Info("Initialize")

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "mdx",
			Version: &version,
		},
	}, nil
}

func (s *Server) onInitialized(ctx *glsp.Context, params *protocol.InitializedParams) error {
	slog.Info("Initialized")
	return nil
}

func (s *Server) onShutdown(ctx *glsp.Context) error {
	slog.Info("shitdown")
	s.shutdown.Store(true)
	if err := s.conn.Close(); err != nil {
		slog.Error("db close", "err", err)
	}
	return nil
}

func (s *Server) onExit(ctx *glsp.Context) error {
	if s.shutdown.Load() {
		os.Exit(0)
	}
	os.Exit(1)
	return nil
}
