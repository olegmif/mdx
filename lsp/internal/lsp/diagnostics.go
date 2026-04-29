package lsp

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/olegmif/mdx/lsp/internal/index"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func Build(links []index.ResolvedLink) []protocol.Diagnostic {
	out := make([]protocol.Diagnostic, 0, len(links))
	for _, l := range links {
		_, err := os.Lstat(l.TargetPath)
		if err == nil {
			continue
		}
		if !errors.Is(err, fs.ErrNotExist) {
			continue
		}
		line := protocol.UInteger(l.Line - 1)
		col := protocol.UInteger(l.Col - 1)
		sev := protocol.DiagnosticSeverityWarning
		src := "mdx"
		out = append(out, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: line, Character: col},
				End:   protocol.Position{Line: line, Character: col},
			},
			Severity: &sev,
			Source:   &src,
			Message:  fmt.Sprintf("broken link: %s", l.RawTarget),
		})
	}
	return out
}
