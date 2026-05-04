package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"github.com/olegmif/mdx/lsp/internal/config"
)

const defaultHTTPTimeout = 60 * time.Second

// ModelClient is an HTTP client for one embedding model. It dispatches
// requests according to cfg.EndpointKind ("openai" | "llama-cpp" | "tei").
type ModelClient struct {
	cfg  config.ModelConfig
	http *http.Client
}

// NewModelClient builds a ModelClient. The HTTP timeout is a package-level
// constant for now; a per-model timeout knob is out of M0 scope.
func NewModelClient(cfg config.ModelConfig) *ModelClient {
	return &ModelClient{
		cfg:  cfg,
		http: &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// Embed prefixes every text with prefix, slices the result into batches of
// cfg.BatchSize and returns one vector per input in the same order.
//
// prefix is concatenated as-is (prefix + text); for asymmetric retrieval
// models the prefix itself carries the necessary separator. See
// config.ModelConfig.QueryPrefix / DocumentPrefix.
func (c *ModelClient) Embed(ctx context.Context, texts []string, prefix string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += c.cfg.BatchSize {
		end := start + c.cfg.BatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := make([]string, end-start)
		for i := range batch {
			batch[i] = prefix + texts[start+i]
		}
		vecs, err := c.embedBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
	}
	return out, nil
}

func (c *ModelClient) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	switch c.cfg.EndpointKind {
	case "openai":
		return c.embedOpenAI(ctx, texts)
	case "llama-cpp":
		return c.embedLlamaCPP(ctx, texts)
	case "tei":
		return c.embedTEI(ctx, texts)
	default:
		return nil, fmt.Errorf("model %q: unsupported endpoint_kind %q",
			c.cfg.Name, c.cfg.EndpointKind)
	}
}

// --- openai ----------------------------------------------------------------

type openaiRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openaiResponse struct {
	Data []openaiEmbedding `json:"data"`
}

type openaiEmbedding struct {
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

func (c *ModelClient) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(openaiRequest{
		Model: c.cfg.APIModelName,
		Input: texts,
	})
	if err != nil {
		return nil, fmt.Errorf("model %q: marshal: %w", c.cfg.Name, err)
	}
	var resp openaiResponse
	if err := c.doJSON(ctx, c.cfg.Endpoint, body, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("model %q: got %d embeddings for %d inputs",
			c.cfg.Name, len(resp.Data), len(texts))
	}
	// Сервер не обязан отдавать embeddings в порядке индексов — сортируем
	// сами, чтобы вектор out[i] однозначно соответствовал texts[i].
	sort.SliceStable(resp.Data, func(i, j int) bool {
		return resp.Data[i].Index < resp.Data[j].Index
	})
	out := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

// --- llama-cpp -------------------------------------------------------------

type llamaRequest struct {
	Content string `json:"content"`
}

type llamaResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (c *ModelClient) embedLlamaCPP(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		body, err := json.Marshal(llamaRequest{Content: text})
		if err != nil {
			return nil, fmt.Errorf("model %q: marshal: %w", c.cfg.Name, err)
		}
		var resp llamaResponse
		if err := c.doJSON(ctx, c.cfg.Endpoint, body, &resp); err != nil {
			return nil, err
		}
		out[i] = resp.Embedding
	}
	return out, nil
}

// --- tei -------------------------------------------------------------------

type teiRequest struct {
	Inputs []string `json:"inputs"`
}

func (c *ModelClient) embedTEI(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(teiRequest{Inputs: texts})
	if err != nil {
		return nil, fmt.Errorf("model %q: marshal: %w", c.cfg.Name, err)
	}
	var resp [][]float32
	if err := c.doJSON(ctx, c.cfg.Endpoint, body, &resp); err != nil {
		return nil, err
	}
	if len(resp) != len(texts) {
		return nil, fmt.Errorf("model %q: got %d embeddings for %d inputs",
			c.cfg.Name, len(resp), len(texts))
	}
	return resp, nil
}

// --- shared transport ------------------------------------------------------

func (c *ModelClient) doJSON(ctx context.Context, url string, body []byte, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("model %q: build request: %w", c.cfg.Name, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("model %q: http: %w", c.cfg.Name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("model %q: http %d: %s",
			c.cfg.Name, resp.StatusCode, firstLine(snippet))
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("model %q: decode response: %w", c.cfg.Name, err)
	}
	return nil
}

func firstLine(b []byte) string {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}
