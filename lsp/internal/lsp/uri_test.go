package lsp

import "testing"

func TestURIRoundTrip(t *testing.T) {
	cases := []struct {
		path string
		uri  string
	}{
		{"/home/oleg/notes/x.md", "file:///home/oleg/notes/x.md"},
		{"/tmp/with space.md", "file:///tmp/with%20space.md"},
		{"/", "file:///"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			if got := PathToURI(c.path); got != c.uri {
				t.Errorf("PathToURI(%q) = %q, want %q", c.path, got, c.uri)
			}
			got, err := URIToPath(c.uri)
			if err != nil {
				t.Fatalf("URIToPath(%q): %v", c.uri, err)
			}
			if got != c.path {
				t.Errorf("URIToPath(%q) = %q, want %q", c.uri, got, c.path)
			}
		})
	}
}

func TestURIToPathRejectsNonFile(t *testing.T) {
	for _, uri := range []string{
		"http://example.com/x.md",
		"https://example.com/x.md",
		"mailto:foo@example.com",
	} {
		if _, err := URIToPath(uri); err == nil {
			t.Errorf("URIToPath(%q): want error, got nil", uri)
		}
	}
}
