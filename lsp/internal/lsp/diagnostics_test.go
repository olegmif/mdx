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
		{Text: "ok", RawTarget: "./exists.md", TargetPath: exists, Line: 1, Col: 1},
		{Text: "broken", RawTarget: "./missing.md", TargetPath: missing, Line: 3, Col: 5},
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

func TestBuildEmptyText(t *testing.T) {
	tmp := t.TempDir()
	exists := filepath.Join(tmp, "exists.md")
	if err := os.WriteFile(exists, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(tmp, "missing.md")

	links := []index.ResolvedLink{
		{Text: "", RawTarget: "./exists.md", TargetPath: exists, Line: 2, Col: 1},
		{Text: "", RawTarget: "./missing.md", TargetPath: missing, Line: 4, Col: 1},
	}

	got := Build(links)
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3 (empty + empty+broken)", len(got))
	}
	// Порядок: для links[0] — один empty-title; для links[1] — empty-title + broken
	if got[0].Message != "empty link title: [](./exists.md)" {
		t.Errorf("got[0].Message = %q", got[0].Message)
	}
	if got[1].Message != "empty link title: [](./missing.md)" {
		t.Errorf("got[1].Message = %q", got[1].Message)
	}
	if got[2].Message != "broken link: ./missing.md" {
		t.Errorf("got[2].Message = %q", got[2].Message)
	}
}
