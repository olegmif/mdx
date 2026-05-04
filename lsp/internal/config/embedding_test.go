package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEmbedding(t *testing.T) {
	cases := []struct {
		name      string
		yaml      string
		wantErr   bool
		errSubstr string
		check     func(t *testing.T, cfg EmbeddingConfig)
	}{
		{
			name: "single model, no default_for_search",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
`,
			check: func(t *testing.T, cfg EmbeddingConfig) {
				if len(cfg.Models) != 1 {
					t.Fatalf("models = %d, want 1", len(cfg.Models))
				}
				if cfg.Models[0].Name != "m1" {
					t.Errorf("name = %q, want m1", cfg.Models[0].Name)
				}
			},
		},
		{
			name: "two models, one default",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
    default_for_search: true
  - name: m2
    endpoint: http://127.0.0.1:8889/embedding
    endpoint_kind: llama-cpp
    api_model_name: M2
    dim: 16
    distance: cosine
    batch_size: 1
`,
			check: func(t *testing.T, cfg EmbeddingConfig) {
				if len(cfg.Models) != 2 {
					t.Fatalf("models = %d, want 2", len(cfg.Models))
				}
				if !cfg.Models[0].DefaultForSearch {
					t.Error("expected models[0].default_for_search = true")
				}
				if cfg.Models[1].DefaultForSearch {
					t.Error("expected models[1].default_for_search = false")
				}
			},
		},
		{
			name: "two models, no default",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
  - name: m2
    endpoint: http://127.0.0.1:8889/embedding
    endpoint_kind: llama-cpp
    api_model_name: M2
    dim: 16
    distance: cosine
    batch_size: 1
`,
			wantErr:   true,
			errSubstr: "default_for_search",
		},
		{
			name: "two models, both default",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
    default_for_search: true
  - name: m2
    endpoint: http://127.0.0.1:8889/embedding
    endpoint_kind: llama-cpp
    api_model_name: M2
    dim: 16
    distance: cosine
    batch_size: 1
    default_for_search: true
`,
			wantErr:   true,
			errSubstr: "default_for_search",
		},
		{
			name: "unknown endpoint_kind",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: vllm
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
`,
			wantErr:   true,
			errSubstr: "endpoint_kind",
		},
		{
			name: "distance dot",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: dot
    batch_size: 4
`,
			wantErr:   true,
			errSubstr: "cosine",
		},
		{
			name: "dim zero",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 0
    distance: cosine
    batch_size: 4
`,
			wantErr:   true,
			errSubstr: "dim",
		},
		{
			name: "duplicate name",
			yaml: `
qdrant_url: http://127.0.0.1:6333
collection: mdx
models:
  - name: m1
    endpoint: http://127.0.0.1:8888/v1/embeddings
    endpoint_kind: openai
    api_model_name: M1
    dim: 8
    distance: cosine
    batch_size: 4
    default_for_search: true
  - name: m1
    endpoint: http://127.0.0.1:8889/embedding
    endpoint_kind: llama-cpp
    api_model_name: M2
    dim: 16
    distance: cosine
    batch_size: 1
`,
			wantErr:   true,
			errSubstr: "duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "embedding.yaml")
			if err := os.WriteFile(p, []byte(tc.yaml), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cfg, warnings, err := LoadEmbedding(p)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("LoadEmbedding: want error, got nil")
				}
				if tc.errSubstr != "" && !strings.Contains(err.Error(), tc.errSubstr) {
					t.Errorf("err = %q, want substring %q", err.Error(), tc.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadEmbedding: %v", err)
			}
			if warnings != nil {
				t.Errorf("warnings = %v, want nil on M0", warnings)
			}
			if tc.check != nil {
				tc.check(t, cfg)
			}
		})
	}
}

func TestLoadEmbeddingMissingFile(t *testing.T) {
	_, _, err := LoadEmbedding(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("LoadEmbedding: want error for missing file, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want errors.Is(err, fs.ErrNotExist)", err)
	}
}

func TestResolveEmbeddingPathOverride(t *testing.T) {
	got, err := ResolveEmbeddingPath("/explicit/embedding.yaml")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "/explicit/embedding.yaml" {
		t.Errorf("got %q", got)
	}
}

func TestResolveEmbeddingPathEnv(t *testing.T) {
	t.Setenv("MDX_EMBEDDING_CONFIG", "/env/embedding.yaml")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	got, err := ResolveEmbeddingPath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got != "/env/embedding.yaml" {
		t.Errorf("got %q", got)
	}
}

func TestResolveEmbeddingPathXDG(t *testing.T) {
	t.Setenv("MDX_EMBEDDING_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "/xdg/cfg")
	got, err := ResolveEmbeddingPath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := filepath.Join("/xdg/cfg", "mdx", "embedding.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveEmbeddingPathHomeFallback(t *testing.T) {
	t.Setenv("MDX_EMBEDDING_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	got, err := ResolveEmbeddingPath("")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := filepath.Join(home, ".config", "mdx", "embedding.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
