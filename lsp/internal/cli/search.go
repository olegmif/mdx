package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/olegmif/mdx/lsp/internal/config"
	"github.com/olegmif/mdx/lsp/internal/embed"
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

const defaultSearchLimit = 20

// RunSearch resolves the search model, embeds the query (with the model's
// QueryPrefix applied), runs k-NN against Qdrant and returns the hits in
// the order Qdrant produced them. Points whose payload lacks "path" are
// skipped with a stderr warning — they signal a desynchronised collection
// and are useless to the caller.
func RunSearch(ctx context.Context, cfg config.EmbeddingConfig, query string, opts SearchOptions) ([]SearchHit, error) {
	model, err := selectSearchModel(cfg, opts.Model)
	if err != nil {
		return nil, err
	}

	client := embed.NewModelClient(model)
	vectors, err := client.Embed(ctx, []string{query}, model.QueryPrefix)
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("model %q: got %d vectors for 1 query", model.Name, len(vectors))
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	qd := embed.NewQdrantClient(cfg.QdrantURL)
	raw, err := qd.Search(ctx, cfg.Collection, model.Name, vectors[0], limit)
	if err != nil {
		return nil, err
	}

	out := make([]SearchHit, 0, len(raw))
	for _, h := range raw {
		path := payloadString(h.Payload, "path")
		if path == "" {
			fmt.Fprintf(os.Stderr, "mdx: search: point %s has no path in payload, skipped\n", h.ID)
			continue
		}
		out = append(out, SearchHit{
			Path:  path,
			Score: h.Score,
			Title: payloadString(h.Payload, "title"),
		})
	}
	return out, nil
}

// payloadString reads p[key] and returns it if it is a string; missing
// keys, nil maps and non-string values all collapse to "".
func payloadString(p map[string]any, key string) string {
	if v, ok := p[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// FormatText renders hits as one path per line, terminated by \n. An empty
// or nil slice renders to "" — printing it produces no output, which is
// what shell pipelines (xargs/fzf/vim) want for an empty result.
func FormatText(hits []SearchHit) string {
	var b strings.Builder
	for _, h := range hits {
		b.WriteString(h.Path)
		b.WriteByte('\n')
	}
	return b.String()
}

// FormatJSON renders hits as a JSON array of {path,score,title?} objects.
// A nil slice is normalised to [] so the output is never the literal "null".
// The trailing newline is added by the caller.
func FormatJSON(hits []SearchHit) ([]byte, error) {
	if hits == nil {
		hits = []SearchHit{}
	}
	return json.Marshal(hits)
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
