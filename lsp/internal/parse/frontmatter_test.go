package parse

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantFM   map[string]any
		wantBody string
		wantErr  bool
	}{
		{
			name:     "no frontmatter",
			input:    "just some text\n",
			wantFM:   nil,
			wantBody: "just some text\n",
		},
		{
			name:  "frontmatter with tags",
			input: "---\ntitle: Hello\ntags: [a, b]\n---\nbody line\n",
			wantFM: map[string]any{
				"title": "Hello",
				"tags":  []any{"a", "b"},
			},
			wantBody: "body line\n",
		},
		{
			name:     "alternative dots closer",
			input:    "---\nkey: value\n...\nthe body\n",
			wantFM:   map[string]any{"key": "value"},
			wantBody: "the body\n",
		},
		{
			name:    "broken yaml between markers",
			input:   "---\n{ broken\n---\nbody\n",
			wantErr: true,
		},
		{
			name:    "missing closer",
			input:   "---\ntitle: Oops\nbody continues\n",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm, body, err := Parse([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !reflect.DeepEqual(fm, tc.wantFM) {
				t.Errorf("frontmatter = %#v, want %#v", fm, tc.wantFM)
			}
			if string(body) != tc.wantBody {
				t.Errorf("body = %q, want %q", string(body), tc.wantBody)
			}
		})
	}
}
