package cli

import (
	"context"
	"errors"

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
