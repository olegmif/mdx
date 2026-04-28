package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTarget(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}

	cases := []struct {
		name      string
		sourceDir string
		raw       string
		want      string
	}{
		{
			name:      "relative",
			sourceDir: "/notes/sub",
			raw:       "../top.md",
			want:      "/notes/top.md",
		},
		{
			name:      "absolute",
			sourceDir: "/notes/sub",
			raw:       "/abs/path.md",
			want:      "/abs/path.md",
		},
		{
			name:      "tilde",
			sourceDir: "/notes/sub",
			raw:       "~/notes/x.md",
			want:      filepath.Join(home, "notes", "x.md"),
		},
		{
			name:      "dot relative",
			sourceDir: "/notes",
			raw:       "./adjacent.md",
			want:      "/notes/adjacent.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveTarget(tc.sourceDir, tc.raw)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
