package embed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
)

func TestEmbedOpenAIBatchAndPrefix(t *testing.T) {
	var seen openaiRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		// Намеренно отдаём в обратном порядке индексов — Embed должен
		// отсортировать данные перед возвратом.
		resp := openaiResponse{Data: []openaiEmbedding{
			{Index: 1, Embedding: []float32{2, 2}},
			{Index: 0, Embedding: []float32{1, 1}},
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewModelClient(config.ModelConfig{
		Name:         "m",
		Endpoint:     srv.URL,
		EndpointKind: "openai",
		APIModelName: "test-model",
		BatchSize:    8,
	})
	out, err := c.Embed(context.Background(), []string{"a", "b"}, "Q: ")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0][0] != 1 || out[1][0] != 2 {
		t.Errorf("vectors out of order: %v", out)
	}
	if seen.Model != "test-model" {
		t.Errorf("model = %q, want test-model", seen.Model)
	}
	if !equalStr(seen.Input, []string{"Q: a", "Q: b"}) {
		t.Errorf("prefix not applied: %v", seen.Input)
	}
}

func TestEmbedLlamaCPPSequential(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		seen = append(seen, req.Content)
		// Вектор отражает порядок прихода: первое значение = индекс приёма + 1.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llamaResponse{
			Embedding: []float32{float32(len(seen))},
		})
	}))
	defer srv.Close()

	c := NewModelClient(config.ModelConfig{
		Name:         "m",
		Endpoint:     srv.URL,
		EndpointKind: "llama-cpp",
		BatchSize:    4,
	})
	out, err := c.Embed(context.Background(), []string{"x", "y", "z"}, "")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0][0] != 1 || out[1][0] != 2 || out[2][0] != 3 {
		t.Errorf("order broken: %v", out)
	}
	if !equalStr(seen, []string{"x", "y", "z"}) {
		t.Errorf("seen = %v, want [x y z]", seen)
	}
}

func TestEmbedTEIBatchSizes(t *testing.T) {
	var batchSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req teiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode req: %v", err)
		}
		batchSizes = append(batchSizes, len(req.Inputs))
		out := make([][]float32, len(req.Inputs))
		for i := range req.Inputs {
			out[i] = []float32{float32(i)}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	c := NewModelClient(config.ModelConfig{
		Name:         "m",
		Endpoint:     srv.URL,
		EndpointKind: "tei",
		BatchSize:    2,
	})
	// 5 текстов при batch_size=2 → батчи 2, 2, 1.
	out, err := c.Embed(context.Background(), []string{"a", "b", "c", "d", "e"}, "")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(out) != 5 {
		t.Fatalf("len = %d, want 5", len(out))
	}
	if !equalInt(batchSizes, []int{2, 2, 1}) {
		t.Errorf("batches = %v, want [2 2 1]", batchSizes)
	}
}

func TestEmbedHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewModelClient(config.ModelConfig{
		Name:         "broken",
		Endpoint:     srv.URL,
		EndpointKind: "openai",
		APIModelName: "x",
		BatchSize:    4,
	})
	_, err := c.Embed(context.Background(), []string{"a"}, "")
	if err == nil {
		t.Fatal("Embed: want error, got nil")
	}
	if !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want substrings [broken, 500]", err.Error())
	}
}

func TestEmbedInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "not-json")
	}))
	defer srv.Close()

	c := NewModelClient(config.ModelConfig{
		Name:         "broken",
		Endpoint:     srv.URL,
		EndpointKind: "openai",
		APIModelName: "x",
		BatchSize:    4,
	})
	_, err := c.Embed(context.Background(), []string{"a"}, "")
	if err == nil {
		t.Fatal("Embed: want error, got nil")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %q, want substring %q", err.Error(), "broken")
	}
}

func equalStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
