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

func TestOnDidSave(t *testing.T) {
	tmp := t.TempDir()
	mdPath := filepath.Join(tmp, "test.md")
	if err := os.WriteFile(mdPath, []byte("# v1\n[broken](./nope.md)\n"), 0o644); err != nil {
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

	// Первое сохранение — индекс должен появиться.
	if err := s.onDidSave(ctx, &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentUri(PathToURI(mdPath)),
		},
	}); err != nil {
		t.Fatalf("onDidSave: %v", err)
	}

	var hash1 string
	if err := conn.QueryRow(`SELECT content_hash FROM notes WHERE path = ?`, mdPath).Scan(&hash1); err != nil {
		t.Fatalf("read hash1: %v", err)
	}

	// Перезаписываем файл и сохраняем снова — хеш в БД должен обновиться.
	if err := os.WriteFile(mdPath, []byte("# v2\n[broken](./nope.md)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.onDidSave(ctx, &protocol.DidSaveTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentUri(PathToURI(mdPath)),
		},
	}); err != nil {
		t.Fatalf("onDidSave (rerun): %v", err)
	}

	var hash2 string
	if err := conn.QueryRow(`SELECT content_hash FROM notes WHERE path = ?`, mdPath).Scan(&hash2); err != nil {
		t.Fatalf("read hash2: %v", err)
	}
	if hash1 == hash2 {
		t.Errorf("hash unchanged after rewrite: %s", hash1)
	}

	if len(calls) != 2 {
		t.Fatalf("notifications = %d, want 2", len(calls))
	}
	for i, c := range calls {
		pd, ok := c.params.(protocol.PublishDiagnosticsParams)
		if !ok {
			t.Fatalf("call[%d] params type %T", i, c.params)
		}
		if len(pd.Diagnostics) != 1 {
			t.Errorf("call[%d] diagnostics = %d, want 1", i, len(pd.Diagnostics))
		}
	}
}

func TestOnListNotes(t *testing.T) {
	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "mdx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	seed := []db.NoteRecord{
		{Path: "/notes/charlie.md", Title: "Alpha"},
		{Path: "/notes/bravo.md", Title: ""}, // fallback на basename: "bravo"
		{Path: "/notes/delta.md", Title: "Charlie"},
	}
	for _, n := range seed {
		if err := db.UpsertNote(tx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	s := &Server{conn: conn}
	got, err := s.onListNotes(nil)
	if err != nil {
		t.Fatalf("onListNotes: %v", err)
	}

	want := []db.NoteEntry{
		{Path: "/notes/charlie.md", Title: "Alpha"},
		{Path: "/notes/bravo.md", Title: "bravo"},
		{Path: "/notes/delta.md", Title: "Charlie"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestOnSearchByTags(t *testing.T) {
	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "mdx.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	notes := []db.NoteRecord{
		{Path: "/notes/charlie.md", Title: "Alpha"},
		{Path: "/notes/bravo.md", Title: ""}, // fallback на basename: "bravo"
		{Path: "/notes/delta.md", Title: "Charlie"},
		{Path: "/notes/skip.md", Title: "Excluded"}, // помечен draft, должен быть отфильтрован
	}
	for _, n := range notes {
		if err := db.UpsertNote(tx, n); err != nil {
			t.Fatal(err)
		}
	}
	tagsByPath := map[string][]string{
		"/notes/charlie.md": {"mdx"},
		"/notes/bravo.md":   {"mdx"},
		"/notes/delta.md":   {"mdx"},
		"/notes/skip.md":    {"mdx", "draft"},
	}
	for path, tags := range tagsByPath {
		if err := db.ReplaceTags(tx, path, tags); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	s := &Server{conn: conn}
	got, err := s.onSearchByTags(nil, []string{"mdx"}, []string{"draft"})
	if err != nil {
		t.Fatalf("onSearchByTags: %v", err)
	}

	// Сортировка после fallback'а: case-insensitive по title, затем по path.
	// "bravo" (fallback) < "Alpha"? Нет — сравниваем по lower:
	//   alpha < bravo < charlie. Поэтому порядок такой:
	want := []db.NoteEntry{
		{Path: "/notes/charlie.md", Title: "Alpha"},
		{Path: "/notes/bravo.md", Title: "bravo"},
		{Path: "/notes/delta.md", Title: "Charlie"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}
