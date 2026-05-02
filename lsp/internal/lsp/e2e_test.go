package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/olegmif/mdx/lsp/internal/db"
)

func frameWrite(w io.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func frameRead(r *bufio.Reader) ([]byte, error) {
	var n int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err = strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, err
			}
		}
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func TestServerRoundTrip(t *testing.T) {
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
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = inR
	os.Stdout = outW
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	srv := New(conn, nil)
	done := make(chan error, 1)
	go func() {
		done <- srv.RunStdio()
	}()

	reader := bufio.NewReader(outR)

	send := func(payload map[string]any) {
		t.Helper()
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := frameWrite(inW, b); err != nil {
			t.Fatal(err)
		}
	}
	recv := func() []byte {
		t.Helper()
		body, err := frameRead(reader)
		if err != nil {
			t.Fatal(err)
		}
		return body
	}

	// initialize → InitializeResult
	send(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"processId":    1,
			"rootUri":      nil,
			"capabilities": map[string]any{},
		},
	})
	var initResp struct {
		ID     int                       `json:"id"`
		Result protocol.InitializeResult `json:"result"`
	}
	if err := json.Unmarshal(recv(), &initResp); err != nil {
		t.Fatalf("initialize response: %v", err)
	}
	if initResp.ID != 1 {
		t.Errorf("response id = %d, want 1", initResp.ID)
	}
	if initResp.Result.ServerInfo == nil || initResp.Result.ServerInfo.Name != "mdx" {
		t.Errorf("server info = %+v", initResp.Result.ServerInfo)
	}

	// initialized
	send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]any{},
	})

	// didOpen → publishDiagnostics
	send(map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{
				"uri":        PathToURI(mdPath),
				"languageId": "markdown",
				"version":    1,
				"text":       content,
			},
		},
	})
	var diagNotif struct {
		Method string                            `json:"method"`
		Params protocol.PublishDiagnosticsParams `json:"params"`
	}
	if err := json.Unmarshal(recv(), &diagNotif); err != nil {
		t.Fatalf("publishDiagnostics: %v", err)
	}
	if diagNotif.Method != string(protocol.ServerTextDocumentPublishDiagnostics) {
		t.Errorf("method = %q", diagNotif.Method)
	}
	if len(diagNotif.Params.Diagnostics) != 1 {
		t.Errorf("diagnostics = %d, want 1", len(diagNotif.Params.Diagnostics))
	}

	// shutdown → response (null)
	send(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "shutdown",
	})
	if _, err := frameRead(reader); err != nil {
		t.Fatalf("shutdown response: %v", err)
	}

	// EOF on stdin → RunStdio returns
	inW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunStdio: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func TestServerListNotes(t *testing.T) {
	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "mdx.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertNote(tx, db.NoteRecord{Path: "/notes/a.md", Title: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertNote(tx, db.NoteRecord{Path: "/notes/b.md", Title: "Beta"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = inR
	os.Stdout = outW
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	srv := New(conn, nil)
	done := make(chan error, 1)
	go func() { done <- srv.RunStdio() }()

	reader := bufio.NewReader(outR)
	send := func(payload map[string]any) {
		t.Helper()
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := frameWrite(inW, b); err != nil {
			t.Fatal(err)
		}
	}

	send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"processId": 1, "rootUri": nil, "capabilities": map[string]any{}},
	})
	if _, err := frameRead(reader); err != nil {
		t.Fatalf("initialize response: %v", err)
	}

	send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})

	send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "mdx/listNotes", "params": map[string]any{}})
	body, err := frameRead(reader)
	if err != nil {
		t.Fatalf("listNotes response: %v", err)
	}
	var listResp struct {
		ID     int            `json:"id"`
		Result []db.NoteEntry `json:"result"`
	}
	if err := json.Unmarshal(body, &listResp); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, body)
	}
	if listResp.ID != 2 {
		t.Errorf("ID = %d, want 2", listResp.ID)
	}
	if len(listResp.Result) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(listResp.Result), listResp.Result)
	}
	if listResp.Result[0] != (db.NoteEntry{Path: "/notes/a.md", Title: "Alpha"}) {
		t.Errorf("entry[0] = %+v", listResp.Result[0])
	}
	if listResp.Result[1] != (db.NoteEntry{Path: "/notes/b.md", Title: "Beta"}) {
		t.Errorf("entry[1] = %+v", listResp.Result[1])
	}

	send(map[string]any{"jsonrpc": "2.0", "id": 3, "method": "shutdown"})
	if _, err := frameRead(reader); err != nil {
		t.Fatalf("shutdown response: %v", err)
	}
	inW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunStdio: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}

func TestServerSearchByTags(t *testing.T) {
	tmp := t.TempDir()
	conn, err := db.Open(filepath.Join(tmp, "mdx.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(conn); err != nil {
		t.Fatal(err)
	}
	tx, err := conn.Begin()
	if err != nil {
		t.Fatal(err)
	}
	notes := []db.NoteRecord{
		{Path: "/notes/a.md", Title: "Alpha"},
		{Path: "/notes/b.md", Title: "Beta"},
		{Path: "/notes/c.md", Title: "Gamma"},
	}
	for _, n := range notes {
		if err := db.UpsertNote(tx, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ReplaceTags(tx, "/notes/a.md", []string{"go", "mdx"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceTags(tx, "/notes/b.md", []string{"mdx"}); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceTags(tx, "/notes/c.md", []string{"vim"}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdin, origStdout := os.Stdin, os.Stdout
	os.Stdin = inR
	os.Stdout = outW
	t.Cleanup(func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
	})

	srv := New(conn, nil)
	done := make(chan error, 1)
	go func() { done <- srv.RunStdio() }()

	reader := bufio.NewReader(outR)
	send := func(payload map[string]any) {
		t.Helper()
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		if err := frameWrite(inW, b); err != nil {
			t.Fatal(err)
		}
	}

	send(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"processId": 1, "rootUri": nil, "capabilities": map[string]any{}},
	})
	if _, err := frameRead(reader); err != nil {
		t.Fatalf("initialize response: %v", err)
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})

	type resp struct {
		ID     int            `json:"id"`
		Result []db.NoteEntry `json:"result"`
	}
	call := func(id int, include, exclude []string) resp {
		t.Helper()
		send(map[string]any{
			"jsonrpc": "2.0", "id": id, "method": "mdx/searchByTags",
			"params": map[string]any{"include": include, "exclude": exclude},
		})
		body, err := frameRead(reader)
		if err != nil {
			t.Fatalf("searchByTags id=%d: %v", id, err)
		}
		var r resp
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatalf("unmarshal id=%d: %v\nbody: %s", id, err, body)
		}
		if r.ID != id {
			t.Errorf("ID = %d, want %d", r.ID, id)
		}
		return r
	}

	// 1. include=["mdx"] → a.md, b.md.
	r1 := call(2, []string{"mdx"}, []string{})
	if len(r1.Result) != 2 {
		t.Fatalf("call 1: got %d entries, want 2: %+v", len(r1.Result), r1.Result)
	}
	if r1.Result[0] != (db.NoteEntry{Path: "/notes/a.md", Title: "Alpha"}) {
		t.Errorf("call 1 entry[0] = %+v", r1.Result[0])
	}
	if r1.Result[1] != (db.NoteEntry{Path: "/notes/b.md", Title: "Beta"}) {
		t.Errorf("call 1 entry[1] = %+v", r1.Result[1])
	}

	// 2. include=["go","mdx*"], exclude=["vim"] → только a.md (есть и go, и mdx*; нет vim).
	r2 := call(3, []string{"go", "mdx*"}, []string{"vim"})
	if len(r2.Result) != 1 {
		t.Fatalf("call 2: got %d entries, want 1: %+v", len(r2.Result), r2.Result)
	}
	if r2.Result[0] != (db.NoteEntry{Path: "/notes/a.md", Title: "Alpha"}) {
		t.Errorf("call 2 entry[0] = %+v", r2.Result[0])
	}

	// 3. пустые фильтры → все 3 заметки.
	r3 := call(4, []string{}, []string{})
	if len(r3.Result) != 3 {
		t.Fatalf("call 3: got %d entries, want 3: %+v", len(r3.Result), r3.Result)
	}

	send(map[string]any{"jsonrpc": "2.0", "id": 99, "method": "shutdown"})
	if _, err := frameRead(reader); err != nil {
		t.Fatalf("shutdown response: %v", err)
	}
	inW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("RunStdio: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not finish")
	}
}
