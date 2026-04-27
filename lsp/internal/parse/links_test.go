package parse

import (
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []Link
	}{
		{
			name:  "no links",
			input: "just some plain text\nwith no links\n",
			want:  nil,
		},
		{
			name:  "single relative link",
			input: "see [intro](./intro.md) for context\n",
			want: []Link{
				{Text: "intro", RawTarget: "./intro.md", Line: 1, Col: 5},
			},
		},
		{
			name:  "absolute path link",
			input: "[home](/Users/o/notes/home.md)\n",
			want: []Link{
				{Text: "home", RawTarget: "/Users/o/notes/home.md", Line: 1, Col: 1},
			},
		},
		{
			name:  "link with title strips title",
			input: `[doc](./doc.md "the docs")` + "\n",
			want: []Link{
				{Text: "doc", RawTarget: "./doc.md", Line: 1, Col: 1},
			},
		},
		{
			name:  "filters web mail and anchor",
			input: "[a](https://example.com) and [b](#section) and [c](mailto:a@b)\n",
			want:  nil,
		},
		{
			name:  "multiline body multiple links",
			input: "first line\nsecond [hello](./hello.md) line\nthird [bye](./bye.md) line\n",
			want: []Link{
				{Text: "hello", RawTarget: "./hello.md", Line: 2, Col: 8},
				{Text: "bye", RawTarget: "./bye.md", Line: 3, Col: 7},
			},
		},
		{
			name:  "tilde home link",
			input: "[notes](~/notes/index.md)\n",
			want: []Link{
				{Text: "notes", RawTarget: "~/notes/index.md", Line: 1, Col: 1},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractLinks([]byte(tc.input))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
