package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/olegmif/mdx/lsp/internal/db"
)

func TestOnInitialize(t *testing.T) {
	s := &Server{}
	res, err := s.onInitialize(nil, &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("onInitialize: %v", err)
	}
	result, ok := res.(protocol.InitializeResult)
	if !ok {
		t.Fatalf("result type %T, want InitializeResult", res)
	}

	sync, ok := result.Capabilities.TextDocumentSync.(protocol.TextDocumentSyncOptions)
	if !ok {
		t.Fatalf("TextDocumentSync type %T, want TextDocumentSyncOptions", result.Capabilities.TextDocumentSync)
	}
	if sync.OpenClose == nil || !*sync.OpenClose {
		t.Errorf("OpenClose = %v, want true", sync.OpenClose)
	}
	if sync.Change == nil || *sync.Change != protocol.TextDocumentSyncKindNone {
		t.Errorf("Change = %v, want None", sync.Change)
	}
	save, ok := sync.Save.(protocol.SaveOptions)
	if !ok {
		t.Fatalf("Save type %T, want SaveOptions", sync.Save)
	}
	if save.IncludeText == nil || *save.IncludeText {
		t.Errorf("IncludeText = %v, want false", save.IncludeText)
	}

	if result.ServerInfo == nil || result.ServerInfo.Name != "mdx" {
		t.Errorf("ServerInfo = %+v, want Name=mdx", result.ServerInfo)
	}
}

func TestOnDidOpen(t *testing.T) {
	tmp := t.TempDir()
	mdPath := filepath.Join(tmp, "test.md")
	content := "# Test\n\n[broken](./nope.md)\n"
	if err := os.WriteFile(mdPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	conn, err := db.Open(filepath.Join(tmp, "mdx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	type call struct {
		method string
		params any
	}
	var calls []call
	ctx := &glsp.Context{
		Notify: func(method string, params any) {
			calls = append(calls, call{method, params})
		},
	}

	s := &Server{conn: conn}
	if err := s.onDidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:  protocol.DocumentUri(PathToURI(mdPath)),
			Text: content,
		},
	}); err != nil {
		t.Fatalf("onDidOpen: %v", err)
	}

	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM notes WHERE path = ?`, mdPath).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("notes for %s = %d, want 1", mdPath, n)
	}

	if len(calls) != 1 {
		t.Fatalf("notifications = %d, want 1", len(calls))
	}
	if calls[0].method != protocol.ServerTextDocumentPublishDiagnostics {
		t.Errorf("method = %s", calls[0].method)
	}
	pd, ok := calls[0].params.(protocol.PublishDiagnosticsParams)
	if !ok {
		t.Fatalf("params type %T", calls[0].params)
	}
	if len(pd.Diagnostics) != 1 {
		t.Errorf("diagnostics = %d, want 1", len(pd.Diagnostics))
	}
}
