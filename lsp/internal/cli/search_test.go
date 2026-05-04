package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
)

func TestFormatText(t *testing.T) {
	hits := []SearchHit{
		{Path: "a.md", Score: 0.5},
		{Path: "b.md", Score: 0.25, Title: "Beta"},
		{Path: "c.md", Score: 0.125},
	}
	got := FormatText(hits)
	want := "a.md\nb.md\nc.md\n"
	if got != want {
		t.Fatalf("FormatText: got %q, want %q", got, want)
	}
	if got := FormatText(nil); got != "" {
		t.Fatalf("FormatText(nil): got %q, want empty", got)
	}
	if got := FormatText([]SearchHit{}); got != "" {
		t.Fatalf("FormatText([]): got %q, want empty", got)
	}
}

func TestFormatJSON(t *testing.T) {
	// Score values are powers of 1/2 so the float32→float64→float32
	// roundtrip through JSON is bit-exact and direct equality is safe.
	hits := []SearchHit{
		{Path: "a.md", Score: 0.5, Title: "Alpha"},
		{Path: "b.md", Score: 0.25}, // no title — must be omitted
		{Path: "c.md", Score: 0.125, Title: "Gamma"},
	}
	data, err := FormatJSON(hits)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}

	var roundTrip []SearchHit
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal typed: %v", err)
	}
	if len(roundTrip) != len(hits) {
		t.Fatalf("len: got %d, want %d", len(roundTrip), len(hits))
	}
	for i := range hits {
		if roundTrip[i] != hits[i] {
			t.Fatalf("hit %d: got %+v, want %+v", i, roundTrip[i], hits[i])
		}
	}

	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	for _, m := range raw {
		path, _ := m["path"].(string)
		_, hasTitle := m["title"]
		switch path {
		case "b.md":
			if hasTitle {
				t.Fatalf("b.md should omit title, got entry %+v", m)
			}
		case "a.md", "c.md":
			if !hasTitle {
				t.Fatalf("%s should have title, got entry %+v", path, m)
			}
		}
	}

	empty, err := FormatJSON(nil)
	if err != nil {
		t.Fatalf("FormatJSON(nil): %v", err)
	}
	if string(empty) != "[]" {
		t.Fatalf("FormatJSON(nil): got %q, want %q", string(empty), "[]")
	}
}

func TestPayloadString(t *testing.T) {
	p := map[string]any{
		"path":  "notes/foo.md",
		"title": "Foo",
		"mtime": int64(1700000000),
	}
	cases := []struct {
		key  string
		want string
	}{
		{"path", "notes/foo.md"},
		{"title", "Foo"},
		{"mtime", ""},   // non-string value → empty
		{"missing", ""}, // missing key → empty
	}
	for _, tc := range cases {
		if got := payloadString(p, tc.key); got != tc.want {
			t.Fatalf("payloadString(p, %q): got %q, want %q", tc.key, got, tc.want)
		}
	}
	if got := payloadString(nil, "path"); got != "" {
		t.Fatalf("payloadString(nil, ...): got %q, want empty", got)
	}
}

func TestSelectSearchModel(t *testing.T) {
	m1 := config.ModelConfig{Name: "m1"}
	m2default := config.ModelConfig{Name: "m2", DefaultForSearch: true}

	cases := []struct {
		name    string
		cfg     config.EmbeddingConfig
		request string
		want    string
		errSubs string
	}{
		{
			name:    "single model without default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1}},
			request: "",
			want:    "m1",
		},
		{
			name:    "single model with default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, default picked when request empty",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, explicit name wins over default",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "m1",
			want:    "m1",
		},
		{
			name:    "two models, missing name returns error mentioning it",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "ghost",
			errSubs: "ghost",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectSearchModel(tc.cfg, tc.request)
			if tc.errSubs != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.errSubs)
				}
				if !strings.Contains(err.Error(), tc.errSubs) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tc.want {
				t.Fatalf("want model %q, got %q", tc.want, got.Name)
			}
		})
	}
}
