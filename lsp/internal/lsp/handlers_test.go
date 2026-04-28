package lsp

import (
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
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
