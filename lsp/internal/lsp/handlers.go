package lsp

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/olegmif/mdx/lsp/internal/config"
	"github.com/olegmif/mdx/lsp/internal/db"
	"github.com/olegmif/mdx/lsp/internal/index"
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

	slog.Info("initialize")

	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    "mdx",
			Version: &version,
		},
	}, nil
}

func (s *Server) onInitialized(ctx *glsp.Context, params *protocol.InitializedParams) error {
	slog.Info("initialized")
	return nil
}

func (s *Server) onShutdown(ctx *glsp.Context) error {
	slog.Info("shutdown")
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

func (s *Server) onDidOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	path, err := URIToPath(uri)
	if err != nil {
		slog.Error("didOpen: uri", "uri", uri, "err", err)
		return nil
	}
	if config.IsIgnored(path, s.ignore) {
		slog.Debug("path ignored", "event", "didOpen", "path", path)
		return nil
	}
	slog.Info("didOpen", "path", path)

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := index.IndexBytes(context.Background(), s.conn, path, []byte(params.TextDocument.Text))
	if err != nil {
		slog.Error("didOpen: index", "path", path, "err", err)
		publishDiagnostics(ctx, uri, nil)
		return nil
	}

	publishDiagnostics(ctx, uri, Build(result.Links))
	return nil
}

func publishDiagnostics(ctx *glsp.Context, uri string, diags []protocol.Diagnostic) {
	if diags == nil {
		diags = []protocol.Diagnostic{}
	}
	ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func (s *Server) onDidSave(ctx *glsp.Context, params *protocol.DidSaveTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	path, err := URIToPath(uri)
	if err != nil {
		slog.Error("didSave: uri", "uri", uri, "err", err)
		return nil
	}
	if config.IsIgnored(path, s.ignore) {
		slog.Debug("path ignored", "event", "didSave", "path", path)
		return nil
	}
	slog.Info("didSave", "path", path)

	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := index.IndexFile(context.Background(), s.conn, path)
	if err != nil {
		slog.Error("didSave: index", "path", path, "err", err)
		publishDiagnostics(ctx, uri, nil)
		return nil
	}

	publishDiagnostics(ctx, uri, Build(result.Links))
	return nil
}

func (s *Server) onListNotes(ctx *glsp.Context) ([]db.NoteEntry, error) {
	entries, err := db.ListNotes(s.conn)
	if err != nil {
		return nil, err
	}
	// Если в БД title пустой (frontmatter без title) — подставляем basename
	// без расширения, чтобы picker не показывал пустую строку и чтобы
	// формируемая ссылка не получалась как "[](path)".
	for i := range entries {
		if entries[i].Title == "" {
			entries[i].Title = strings.TrimSuffix(
				filepath.Base(entries[i].Path), ".md")
		}
	}
	// Сортировка после fallback'а: case-insensitive по заголовку, потом по пути.
	sort.Slice(entries, func(i, j int) bool {
		ti := strings.ToLower(entries[i].Title)
		tj := strings.ToLower(entries[j].Title)
		if ti != tj {
			return ti < tj
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (s *Server) onSearchByTags(ctx *glsp.Context, include []string, exclude []string) ([]db.NoteEntry, error) {
	entries, err := db.SearchByTags(s.conn, include, exclude)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Title == "" {
			entries[i].Title = strings.TrimSuffix(filepath.Base(entries[i].Path), ".md")
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		ti := strings.ToLower(entries[i].Title)
		tj := strings.ToLower(entries[j].Title)
		if ti != tj {
			return ti < tj
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func (s *Server) onQuery(ctx *glsp.Context, sqlStr string, args []any) ([]map[string]any, error) {
	return db.Query(s.conn, sqlStr, args)
}
