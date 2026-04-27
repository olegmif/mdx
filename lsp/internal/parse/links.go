package parse

import (
	"regexp"
	"strings"
)

// Link describes one outgoing markdown link extracted from a body.
// Line and Col are 1-based; Col is in bytes, not runes.
type Link struct {
	Text      string
	RawTarget string
	Line      int
	Col       int
}

// linkRe matches a markdown inline link [text](target) with an optional
// "title". Captures: 1 = text, 2 = target.
var linkRe = regexp.MustCompile(`\[([^\]\n]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)

// nonFileSchemes lists target prefixes that point to something other than
// a local file. Such links are dropped by Extract.
var nonFileSchemes = []string{
	"http://",
	"https://",
	"mailto:",
	"ftp://",
	"tel:",
	"#",
}

// ExtractLinks returns all file-targeting markdown links found in body.
// Targets are returned verbatim (raw), without resolution to absolute paths.
func ExtractLinks(body []byte) []Link {
	matches := linkRe.FindAllSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil
	}

	var out []Link
	for _, m := range matches {
		fullStart := m[0]
		text := string(body[m[2]:m[3]])
		target := string(body[m[4]:m[5]])

		if isNonFile(target) {
			continue
		}

		line, col := lineCol(body, fullStart)
		out = append(out, Link{
			Text:      text,
			RawTarget: target,
			Line:      line,
			Col:       col,
		})
	}

	return out
}

func isNonFile(target string) bool {
	for _, s := range nonFileSchemes {
		if strings.HasPrefix(target, s) {
			return true
		}
	}
	return false
}

// lineCol converts a byte offset into 1-based (line, col) within body.
func lineCol(body []byte, offset int) (line, col int) {
	line, col = 1, 1
	for i := 0; i < offset && i < len(body); i++ {
		if body[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
