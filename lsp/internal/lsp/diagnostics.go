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
		line := protocol.UInteger(l.Line - 1)
		col := protocol.UInteger(l.Col - 1)
		src := "mdx"
		rng := protocol.Range{
			Start: protocol.Position{Line: line, Character: col},
			End:   protocol.Position{Line: line, Character: col},
		}

		// Empty link text: ссылка вида [](target). Conceal скрывает её
		// целиком (нечего показывать в качестве text), поэтому помечаем
		// как warning, чтобы пользователь её увидел и починил вручную.
		if l.Text == "" {
			sev := protocol.DiagnosticSeverityWarning
			out = append(out, protocol.Diagnostic{
				Range:    rng,
				Severity: &sev,
				Source:   &src,
				Message:  fmt.Sprintf("empty link title: [](%s)", l.RawTarget),
			})
		}

		// Broken target: целевой файл не существует.
		if _, err := os.Lstat(l.TargetPath); err != nil && errors.Is(err, fs.ErrNotExist) {
			sev := protocol.DiagnosticSeverityWarning
			out = append(out, protocol.Diagnostic{
				Range:    rng,
				Severity: &sev,
				Source:   &src,
				Message:  fmt.Sprintf("broken link: %s", l.RawTarget),
			})
		}
	}
	return out
}
