package lsp

import (
	"fmt"
	"net/url"
	"path/filepath"
)

func URIToPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("parse uri: %w", err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("unsupported scheme %q (want file)", u.Scheme)
	}
	return filepath.FromSlash(u.Path), nil
}

func PathToURI(path string) string {
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	return u.String()
}
