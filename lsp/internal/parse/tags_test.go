package parse

import (
	"reflect"
	"testing"
)

func TestExtractTags(t *testing.T) {
	cases := []struct {
		name        string
		frontmatter map[string]any
		body        string
		want        []string
	}{
		{
			name: "empty",
			want: nil,
		},
		{
			name: "frontmatter array",
			frontmatter: map[string]any{
				"tags": []any{"go", "notes", "go"},
			},
			want: []string{"go", "notes"},
		},
		{
			name: "frontmatter comma string",
			frontmatter: map[string]any{
				"tags": "go, notes ,  rust",
			},
			want: []string{"go", "notes", "rust"},
		},
		{
			name: "body inline tags",
			body: "intro #go #notes/sub\nmore #go again\n",
			want: []string{"go", "notes/sub"},
		},
		{
			name: "merge frontmatter and body",
			frontmatter: map[string]any{
				"tags": []any{"frontmatter-only"},
			},
			body: "see #body-only\n",
			want: []string{"frontmatter-only", "body-only"},
		},
		{
			name: "no false positives in word context",
			body: "look at issue#123 and notthe#tag here\n",
			want: nil,
		},
		{
			name: "frontmatter array strips leading #",
			frontmatter: map[string]any{
				"tags": []any{"#gtd/reference", "go", "#notes/sub"},
			},
			want: []string{"gtd/reference", "go", "notes/sub"},
		},
		{
			name: "frontmatter comma strips leading #",
			frontmatter: map[string]any{
				"tags": "#go, notes ,  #rust",
			},
			want: []string{"go", "notes", "rust"},
		},
		{
			name: "frontmatter # alone is dropped",
			frontmatter: map[string]any{
				"tags": []any{"#", "go"},
			},
			want: []string{"go"},
		},
		{
			name: "frontmatter #foo and body #foo dedup to single foo",
			frontmatter: map[string]any{
				"tags": []any{"#shared"},
			},
			body: "see #shared again\n",
			want: []string{"shared"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractTags(tc.frontmatter, []byte(tc.body))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
