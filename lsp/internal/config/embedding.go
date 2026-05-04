package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EmbeddingConfig is the parsed contents of ~/.config/mdx/embedding.yaml.
type EmbeddingConfig struct {
	QdrantURL  string        `yaml:"qdrant_url"`
	Collection string        `yaml:"collection"`
	Models     []ModelConfig `yaml:"models"`
}

// ModelConfig describes one embedding model addressable by name.
type ModelConfig struct {
	Name             string `yaml:"name"`
	Endpoint         string `yaml:"endpoint"`
	EndpointKind     string `yaml:"endpoint_kind"`
	APIModelName     string `yaml:"api_model_name"`
	Dim              int    `yaml:"dim"`
	Distance         string `yaml:"distance"`
	QueryPrefix      string `yaml:"query_prefix"`
	DocumentPrefix   string `yaml:"document_prefix"`
	BatchSize        int    `yaml:"batch_size"`
	DefaultForSearch bool   `yaml:"default_for_search"`
}

// ResolveEmbeddingPath picks where the embedding config file lives.
// Precedence: explicit override -> MDX_EMBEDDING_CONFIG env ->
// $XDG_CONFIG_HOME/mdx/embedding.yaml -> $HOME/.config/mdx/embedding.yaml.
// The path is returned even if the file does not exist; LoadEmbedding
// signals that case via errors.Is(err, fs.ErrNotExist).
func ResolveEmbeddingPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if env := os.Getenv("MDX_EMBEDDING_CONFIG"); env != "" {
		return env, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "mdx", "embedding.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "mdx", "embedding.yaml"), nil
}

// LoadEmbedding reads the embedding config at path, validates it and
// returns the parsed structure. If the file does not exist the returned
// error wraps fs.ErrNotExist; the caller decides whether that is fatal.
// The warnings slice is reserved for future non-fatal parse messages and
// is always nil on M0.
func LoadEmbedding(path string) (EmbeddingConfig, []string, error) {
	var cfg EmbeddingConfig

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil, err
		}
		return cfg, nil, fmt.Errorf("read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return EmbeddingConfig{}, nil, fmt.Errorf("parse %s: %w", path, err)
	}

	if err := validateEmbedding(&cfg); err != nil {
		return EmbeddingConfig{}, nil, err
	}
	return cfg, nil, nil
}

func validateEmbedding(cfg *EmbeddingConfig) error {
	if cfg.QdrantURL == "" {
		return errors.New("qdrant_url is empty")
	}
	if u, err := url.Parse(cfg.QdrantURL); err != nil || u.Scheme == "" {
		return fmt.Errorf("qdrant_url %q is not a valid URL", cfg.QdrantURL)
	}
	if cfg.Collection == "" {
		return errors.New("collection is empty")
	}
	if len(cfg.Models) == 0 {
		return errors.New("at least one model must be configured")
	}

	names := make(map[string]struct{}, len(cfg.Models))
	defaults := 0
	for i := range cfg.Models {
		m := &cfg.Models[i]
		if err := validateModel(m); err != nil {
			return fmt.Errorf("model[%d]: %w", i, err)
		}
		if _, dup := names[m.Name]; dup {
			return fmt.Errorf("model[%d]: duplicate name %q", i, m.Name)
		}
		names[m.Name] = struct{}{}
		if m.DefaultForSearch {
			defaults++
		}
	}

	if defaults > 1 {
		return fmt.Errorf("more than one model marked default_for_search: true (have %d)", defaults)
	}
	if defaults == 0 && len(cfg.Models) > 1 {
		return errors.New("multiple models configured but none marked default_for_search: true")
	}
	return nil
}

func validateModel(m *ModelConfig) error {
	if m.Name == "" {
		return errors.New("name is empty")
	}
	if m.Endpoint == "" {
		return errors.New("endpoint is empty")
	}
	switch m.EndpointKind {
	case "openai", "llama-cpp", "tei":
		// ok
	case "":
		return errors.New("endpoint_kind is empty")
	default:
		return fmt.Errorf("endpoint_kind %q is not one of: openai, llama-cpp, tei", m.EndpointKind)
	}
	if m.APIModelName == "" {
		return errors.New("api_model_name is empty")
	}
	if m.Dim <= 0 {
		return fmt.Errorf("dim must be > 0, got %d", m.Dim)
	}
	if m.Distance != "cosine" {
		return fmt.Errorf("distance %q: only cosine is supported in M0", m.Distance)
	}
	if m.BatchSize <= 0 {
		return fmt.Errorf("batch_size must be > 0, got %d", m.BatchSize)
	}
	return nil
}
