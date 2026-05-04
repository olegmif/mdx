package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
)

// --- search mocks --------------------------------------------------------

// searchRequestSnapshot — то, что мок Qdrant вытащил из тела запроса
// /points/search. Имена полей повторяют JSON для удобства ассертов.
type searchRequestSnapshot struct {
	VectorName  string
	Vector      []float32
	Limit       int
	WithPayload bool
}

// mockQdrantSearchState хранит ответ, который мок будет возвращать на
// /points/search, и записывает все увиденные запросы.
type mockQdrantSearchState struct {
	mu       sync.Mutex
	requests []searchRequestSnapshot
	response []map[string]any // массив result[] из ответа Qdrant
}

// newMockQdrantSearch поднимает httptest-сервер, обслуживающий только
// POST /collections/{collection}/points/search.
func newMockQdrantSearch(t *testing.T, collection string) (*httptest.Server, *mockQdrantSearchState) {
	t.Helper()
	state := &mockQdrantSearchState{}
	mux := http.NewServeMux()
	mux.HandleFunc("/collections/"+collection+"/points/search", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Vector struct {
				Name   string    `json:"name"`
				Vector []float32 `json:"vector"`
			} `json:"vector"`
			Limit       int  `json:"limit"`
			WithPayload bool `json:"with_payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode search: %v", err)
		}
		state.mu.Lock()
		state.requests = append(state.requests, searchRequestSnapshot{
			VectorName:  body.Vector.Name,
			Vector:      body.Vector.Vector,
			Limit:       body.Limit,
			WithPayload: body.WithPayload,
		})
		resp := state.response
		state.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"result": resp,
			"status": "ok",
		})
	})
	return httptest.NewServer(mux), state
}

// failingServer падает в t.Fatalf на любой запрос — для проверки, что
// клиент до сети вообще не дошёл.
func failingServer(t *testing.T, label string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected %s request: %s %s", label, r.Method, r.URL.Path)
	}))
}

func TestFormatText(t *testing.T) {
	hits := []SearchHit{
		{Path: "a.md", Score: 0.5},
		{Path: "b.md", Score: 0.25, Title: "Beta"},
		{Path: "c.md", Score: 0.125},
	}
	got := FormatText(hits)
	want := "a.md\nb.md\nc.md\n"
	if got != want {
		t.Fatalf("FormatText: got %q, want %q", got, want)
	}
	if got := FormatText(nil); got != "" {
		t.Fatalf("FormatText(nil): got %q, want empty", got)
	}
	if got := FormatText([]SearchHit{}); got != "" {
		t.Fatalf("FormatText([]): got %q, want empty", got)
	}
}

func TestFormatJSON(t *testing.T) {
	// Score values are powers of 1/2 so the float32→float64→float32
	// roundtrip through JSON is bit-exact and direct equality is safe.
	hits := []SearchHit{
		{Path: "a.md", Score: 0.5, Title: "Alpha"},
		{Path: "b.md", Score: 0.25}, // no title — must be omitted
		{Path: "c.md", Score: 0.125, Title: "Gamma"},
	}
	data, err := FormatJSON(hits)
	if err != nil {
		t.Fatalf("FormatJSON: %v", err)
	}

	var roundTrip []SearchHit
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal typed: %v", err)
	}
	if len(roundTrip) != len(hits) {
		t.Fatalf("len: got %d, want %d", len(roundTrip), len(hits))
	}
	for i := range hits {
		if roundTrip[i] != hits[i] {
			t.Fatalf("hit %d: got %+v, want %+v", i, roundTrip[i], hits[i])
		}
	}

	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal raw: %v", err)
	}
	for _, m := range raw {
		path, _ := m["path"].(string)
		_, hasTitle := m["title"]
		switch path {
		case "b.md":
			if hasTitle {
				t.Fatalf("b.md should omit title, got entry %+v", m)
			}
		case "a.md", "c.md":
			if !hasTitle {
				t.Fatalf("%s should have title, got entry %+v", path, m)
			}
		}
	}

	empty, err := FormatJSON(nil)
	if err != nil {
		t.Fatalf("FormatJSON(nil): %v", err)
	}
	if string(empty) != "[]" {
		t.Fatalf("FormatJSON(nil): got %q, want %q", string(empty), "[]")
	}
}

func TestPayloadString(t *testing.T) {
	p := map[string]any{
		"path":  "notes/foo.md",
		"title": "Foo",
		"mtime": int64(1700000000),
	}
	cases := []struct {
		key  string
		want string
	}{
		{"path", "notes/foo.md"},
		{"title", "Foo"},
		{"mtime", ""},   // non-string value → empty
		{"missing", ""}, // missing key → empty
	}
	for _, tc := range cases {
		if got := payloadString(p, tc.key); got != tc.want {
			t.Fatalf("payloadString(p, %q): got %q, want %q", tc.key, got, tc.want)
		}
	}
	if got := payloadString(nil, "path"); got != "" {
		t.Fatalf("payloadString(nil, ...): got %q, want empty", got)
	}
}

func TestSelectSearchModel(t *testing.T) {
	m1 := config.ModelConfig{Name: "m1"}
	m2default := config.ModelConfig{Name: "m2", DefaultForSearch: true}

	cases := []struct {
		name    string
		cfg     config.EmbeddingConfig
		request string
		want    string
		errSubs string
	}{
		{
			name:    "single model without default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1}},
			request: "",
			want:    "m1",
		},
		{
			name:    "single model with default, empty request",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, default picked when request empty",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "",
			want:    "m2",
		},
		{
			name:    "two models, explicit name wins over default",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "m1",
			want:    "m1",
		},
		{
			name:    "two models, missing name returns error mentioning it",
			cfg:     config.EmbeddingConfig{Models: []config.ModelConfig{m1, m2default}},
			request: "ghost",
			errSubs: "ghost",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectSearchModel(tc.cfg, tc.request)
			if tc.errSubs != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.errSubs)
				}
				if !strings.Contains(err.Error(), tc.errSubs) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.errSubs)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tc.want {
				t.Fatalf("want model %q, got %q", tc.want, got.Name)
			}
		})
	}
}

// --- end-to-end RunSearch ------------------------------------------------

func TestRunSearchEndToEnd(t *testing.T) {
	qd, qdState := newMockQdrantSearch(t, "mdx")
	defer qd.Close()
	em, emState := newMockOpenAI(t)
	defer em.Close()

	// Score-значения — степени 1/2 (0.5, 0.25, 0.125), чтобы roundtrip
	// float32 ↔ float64 ↔ float32 был бит-в-бит точным.
	qdState.mu.Lock()
	qdState.response = []map[string]any{
		{"id": "p1", "score": 0.5, "payload": map[string]any{"path": "a.md", "title": "Alpha"}},
		{"id": "p2", "score": 0.25, "payload": map[string]any{"path": "b.md"}},
		{"id": "p3", "score": 0.125, "payload": map[string]any{"path": "c.md", "title": "Gamma"}},
	}
	qdState.mu.Unlock()

	cfg := cfgFor(qd.URL, em.URL, 1)
	cfg.Models[0].QueryPrefix = "Q: "

	hits, err := RunSearch(context.Background(), cfg, "hello", SearchOptions{Limit: 5})
	if err != nil {
		t.Fatalf("RunSearch: %v", err)
	}

	want := []SearchHit{
		{Path: "a.md", Score: 0.5, Title: "Alpha"},
		{Path: "b.md", Score: 0.25},
		{Path: "c.md", Score: 0.125, Title: "Gamma"},
	}
	if len(hits) != len(want) {
		t.Fatalf("got %d hits, want %d", len(hits), len(want))
	}
	for i := range want {
		if hits[i] != want[i] {
			t.Errorf("hit %d: got %+v, want %+v", i, hits[i], want[i])
		}
	}

	qdState.mu.Lock()
	defer qdState.mu.Unlock()
	if len(qdState.requests) != 1 {
		t.Fatalf("Qdrant requests = %d, want 1", len(qdState.requests))
	}
	req := qdState.requests[0]
	if req.VectorName != "m1" {
		t.Errorf("vector.name = %q, want %q", req.VectorName, "m1")
	}
	if req.Limit != 5 {
		t.Errorf("limit = %d, want 5", req.Limit)
	}
	if !req.WithPayload {
		t.Error("with_payload = false, want true")
	}
	if len(req.Vector) == 0 {
		t.Error("vector.vector is empty, want a non-empty embedding")
	}

	emState.mu.Lock()
	defer emState.mu.Unlock()
	if emState.calls != 1 {
		t.Errorf("embed calls = %d, want 1", emState.calls)
	}
	if len(emState.inputs) != 1 || len(emState.inputs[0]) != 1 {
		t.Fatalf("embed inputs = %#v, want one call with one input", emState.inputs)
	}
	if got, want := emState.inputs[0][0], "Q: hello"; got != want {
		t.Errorf("embed input = %q, want %q (query_prefix not applied)", got, want)
	}
}

func TestRunSearchDefaultLimit(t *testing.T) {
	qd, qdState := newMockQdrantSearch(t, "mdx")
	defer qd.Close()
	em, _ := newMockOpenAI(t)
	defer em.Close()

	cfg := cfgFor(qd.URL, em.URL, 1)

	if _, err := RunSearch(context.Background(), cfg, "hello", SearchOptions{Limit: 0}); err != nil {
		t.Fatalf("RunSearch: %v", err)
	}

	qdState.mu.Lock()
	defer qdState.mu.Unlock()
	if len(qdState.requests) != 1 {
		t.Fatalf("Qdrant requests = %d, want 1", len(qdState.requests))
	}
	if got := qdState.requests[0].Limit; got != defaultSearchLimit {
		t.Errorf("limit = %d, want %d (defaultSearchLimit)", got, defaultSearchLimit)
	}
}

func TestRunSearchUnknownModel(t *testing.T) {
	qd := failingServer(t, "qdrant")
	defer qd.Close()
	em := failingServer(t, "embedding")
	defer em.Close()

	cfg := cfgFor(qd.URL, em.URL, 1)
	_, err := RunSearch(context.Background(), cfg, "hello", SearchOptions{Model: "ghost"})
	if err == nil {
		t.Fatal("RunSearch: want error for unknown model, got nil")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("err = %q, want substring %q", err.Error(), "ghost")
	}
}
