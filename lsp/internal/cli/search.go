package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/olegmif/mdx/lsp/internal/config"
)

// SearchOptions collects flags driving a single mdx search call.
type SearchOptions struct {
	Model string // empty = use default_for_search model from config
	Limit int    // <=0 ⇒ defaultSearchLimit applied inside RunSearch
}

// SearchHit is one ranked match returned by RunSearch. JSON tags shape
// the --format json output; "title,omitempty" hides the field for hits
// whose payload had no title.
type SearchHit struct {
	Path  string  `json:"path"`
	Score float32 `json:"score"`
	Title string  `json:"title,omitempty"`
}

// RunSearch is filled in by Step 4 of M1_embeddings.
func RunSearch(ctx context.Context, cfg config.EmbeddingConfig, query string, opts SearchOptions) ([]SearchHit, error) {
	return nil, errors.New("search: not implemented")
}

// selectSearchModel picks one model from cfg according to the M1 rules:
//  1. explicit name → linear lookup, error if absent;
//  2. otherwise → model with default_for_search: true;
//  3. otherwise → the only model in the config;
//  4. otherwise → error.
//
// The last branch is also rejected by config.LoadEmbedding when
// len(Models) > 1; the guard stays here as defence in depth.
func selectSearchModel(cfg config.EmbeddingConfig, name string) (config.ModelConfig, error) {
	if name != "" {
		for _, m := range cfg.Models {
			if m.Name == name {
				return m, nil
			}
		}
		return config.ModelConfig{}, fmt.Errorf("model %q not found in embedding config", name)
	}
	for _, m := range cfg.Models {
		if m.DefaultForSearch {
			return m, nil
		}
	}
	if len(cfg.Models) == 1 {
		return cfg.Models[0], nil
	}
	return config.ModelConfig{}, errors.New("no default search model configured")
}
