package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadIgnoreEmptyPath(t *testing.T) {
	prefixes, warnings, err := LoadIgnore("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if prefixes != nil || warnings != nil {
		t.Errorf("got prefixes=%v warnings=%v, want nil/nil", prefixes, warnings)
	}
}

func TestLoadIgnoreMissingFile(t *testing.T) {
	prefixes, warnings, err := LoadIgnore(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if prefixes != nil || warnings != nil {
		t.Errorf("got prefixes=%v warnings=%v, want nil/nil", prefixes, warnings)
	}
}

func TestLoadIgnoreParse(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "ignore")
	contents := strings.Join([]string{
		"# leading comment",
		"",
		"  /var/log  ",   // absolute, with surrounding whitespace
		"~/.local/state", // tilde-expanded
		"~",              // bare tilde
		"# inline comment line",
		"relative/path", // skipped: not absolute after expansion
		"/foo/bar/",     // trailing slash should be cleaned
	}, "\n") + "\n"
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prefixes, warnings, err := LoadIgnore(p)
	if err != nil {
		t.Fatalf("LoadIgnore: %v", err)
	}

	want := []string{
		"/var/log",
		filepath.Join(home, ".local/state"),
		home,
		"/foo/bar",
	}
	if !reflect.DeepEqual(prefixes, want) {
		t.Errorf("prefixes = %v, want %v", prefixes, want)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want exactly one (relative path)", warnings)
	}
	if !strings.Contains(warnings[0], "relative") {
		t.Errorf("warning = %q, want substring %q", warnings[0], "relative")
	}
}

func TestIsIgnored(t *testing.T) {
	prefixes := []string{"/foo/bar", "/var/log"}

	cases := []struct {
		path string
		want bool
	}{
		{"/foo/bar", true},          // exact
		{"/foo/bar/baz.md", true},   // direct child
		{"/foo/bar/sub/x.md", true}, // deeper
		{"/foo/barbaz", false},      // string-prefix only, not subtree
		{"/foo/barbaz/x.md", false},
		{"/foo", false},                  // ancestor of prefix is not ignored
		{"/var/log/system.md", true},
		{"/var/logger.md", false}, // string-prefix collision again
		{"/etc/passwd", false},
	}

	for _, tc := range cases {
		if got := IsIgnored(tc.path, prefixes); got != tc.want {
			t.Errorf("IsIgnored(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}

	if IsIgnored("/foo/bar", nil) {
		t.Error("IsIgnored with nil prefixes returned true, want false")
	}
}

func TestResolveIgnorePathOverride(t *testing.T) {
	got, err := ResolveIgnorePath("/explicit/path")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "/explicit/path" {
		t.Errorf("got %q, want %q", got, "/explicit/path")
	}
}

func TestResolveIgnorePathEnv(t *testing.T) {
	t.Setenv("MDX_IGNORE", "/env/path")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	got, err := ResolveIgnorePath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "/env/path" {
		t.Errorf("got %q, want %q", got, "/env/path")
	}
}

func TestResolveIgnorePathXDG(t *testing.T) {
	t.Setenv("MDX_IGNORE", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	got, err := ResolveIgnorePath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := filepath.Join("/xdg/cfg", "mdx", "ignore")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveIgnorePathHomeFallback(t *testing.T) {
	t.Setenv("MDX_IGNORE", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	got, err := ResolveIgnorePath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := filepath.Join(home, ".config", "mdx", "ignore")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
