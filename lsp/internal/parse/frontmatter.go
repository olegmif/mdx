package parse

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Parse splits content into a YAML frontmatter map and the body.
//
// Frontmatter is a block at the very start of content, bounded by:
//   - an opener line "---";
//   - a closer line "---" or "...".
//
// With no opener present, returns (nil, content, nil). With opener but no
// closer, returns an error.

func Parse(content []byte) (map[string]any, []byte, error) {
	if !bytes.HasPrefix(content, []byte("---\n")) {
		return nil, content, nil
	}
	const openerLen = 4 // "---n"
	yamlStart := openerLen

	pos := openerLen
	for pos < len(content) {
		nl := bytes.IndexByte(content[pos:], '\n')
		var lineEnd int
		if nl < 0 {
			lineEnd = len(content)
		} else {
			lineEnd = pos + nl
		}

		line := content[pos:lineEnd]

		if bytes.Equal(line, []byte("---")) || bytes.Equal(line, []byte("...")) {
			yamlPart := content[yamlStart:pos]
			var bodyStart int
			if nl < 0 {
				bodyStart = len(content)
			} else {
				bodyStart = lineEnd + 1
			}
			body := content[bodyStart:]

			var fm map[string]any
			if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
				return nil, content, fmt.Errorf("frontmatter: %w", err)
			}
			return fm, body, nil
		}

		if nl < 0 {
			break
		}
		pos = lineEnd + 1
	}

	return nil, content, fmt.Errorf("frintmatter: closing delimiter not found")
}
