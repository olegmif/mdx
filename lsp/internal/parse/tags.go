package parse

import (
	"regexp"
	"strings"
)

// tagInBody matches "#tag" preceded by start of input or a non-word,
// non-& byte. Capture 1 is the tag name itself.
//
// "#tag" must start with a word char and may continue with word chars,
// "/" or "-". The leading non-word constraint avoids matching things
// like "issue#123" or "x#tag" (which are not tags).
var tagInBody = regexp.MustCompile(`(?:^|[^\w&])#([\w][\w/-]*)`)

// ExtractTags returns the deduplicated tags pulled from frontmatter and
// inline #tag occurrences in body. Frontmatter "tags" may be either a
// comma-separated string or a list of strings. Order: frontmatter first,
// body second; within each, in encounter order.
func ExtractTags(frontmatter map[string]any, body []byte) []string {
	seen := make(map[string]struct{})
	var out []string

	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, ok := seen[tag]; ok {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}

	if raw, ok := frontmatter["tags"]; ok {
		switch v := raw.(type) {
		case string:
			for _, t := range strings.Split(v, ",") {
				add(t)
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					add(s)
				}
			}
		}
	}

	for _, m := range tagInBody.FindAllSubmatch(body, -1) {
		add(string(m[1]))
	}

	return out
}
