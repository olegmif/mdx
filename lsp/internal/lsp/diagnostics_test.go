package lsp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/index"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestBuild(t *testing.T) {
	tmp := t.TempDir()
	exists := filepath.Join(tmp, "exists.md")
	if err := os.WriteFile(exists, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "missing.md")

	links := []index.ResolvedLink{
		{RawTarget: "./exists.md", TargetPath: exists, Line: 1, Col: 1},
		{RawTarget: "./missing.md", TargetPath: missing, Line: 3, Col: 5},
	}

	got := Build(links)
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	d := got[0]
	if d.Range.Start.Line != 2 || d.Range.Start.Character != 4 {
		t.Errorf("range start = %+v, want {2,4}", d.Range.Start)
	}
	if d.Severity == nil || *d.Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("severity = %v, want Warning", d.Severity)
	}
	if d.Source == nil || *d.Source != "mdx" {
		t.Errorf("source = %v, want mdx", d.Source)
	}
	if d.Message != "broken link: ./missing.md" {
		t.Errorf("message = %q", d.Message)
	}
}
