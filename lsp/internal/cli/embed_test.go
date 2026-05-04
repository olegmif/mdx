package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/olegmif/mdx/lsp/internal/config"
	"github.com/olegmif/mdx/lsp/internal/db"
)

// --- mocks --------------------------------------------------------------

type mockQdrantState struct {
	mu      sync.Mutex
	created bool
	points  []map[string]any // плоский список всех upserted точек
}

// newMockQdrant returns an httptest server that mimics enough of Qdrant
// REST API for one collection ("mdx"): GET/PUT/PATCH /collections/{name}
// и PUT /collections/{name}/points.
func newMockQdrant(t *testing.T, collection string) (*httptest.Server, *mockQdrantState) {
	t.Helper()
	state := &mockQdrantState{}
	mux := http.NewServeMux()

	mux.HandleFunc("/collections/"+collection, func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()
		switch r.Method {
		case http.MethodGet:
			if !state.created {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"result":{"config":{"params":{"vectors":{"m1":{"size":2,"distance":"Cosine"}}}}},
				"status":"ok"
			}`)
		case http.MethodPut:
			state.created = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":true,"status":"ok"}`)
		case http.MethodPatch:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":true,"status":"ok"}`)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/collections/"+collection+"/points", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Points []map[string]any `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode upsert: %v", err)
		}
		state.mu.Lock()
		state.points = append(state.points, body.Points...)
		state.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{},"status":"ok"}`)
	})

	return httptest.NewServer(mux), state
}

func (s *mockQdrantState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.points = nil
}

func (s *mockQdrantState) pointCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.points)
}

type mockEmbedState struct {
	mu       sync.Mutex
	calls    int
	failOnce bool       // если true, следующий запрос возвращает 500
	inputs   [][]string // записанный массив input каждого вызова (для проверок в search-тестах)
}

// newMockOpenAI поднимает мок embedding-сервера в варианте openai.
// На каждый input возвращает вектор {idx, idx} — детерминированный и
// различимый по индексу в батче.
func newMockOpenAI(t *testing.T) (*httptest.Server, *mockEmbedState) {
	t.Helper()
	state := &mockEmbedState{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		if state.failOnce {
			state.failOnce = false
			state.mu.Unlock()
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		state.calls++
		state.mu.Unlock()

		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode openai: %v", err)
		}
		state.mu.Lock()
		state.inputs = append(state.inputs, append([]string(nil), req.Input...))
		state.mu.Unlock()
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		out := struct {
			Data []item `json:"data"`
		}{}
		for i := range req.Input {
			out.Data = append(out.Data, item{Index: i, Embedding: []float32{float32(i), float32(i)}})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	return srv, state
}

func (s *mockEmbedState) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// --- helpers ------------------------------------------------------------

func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "mdx.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := db.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

// seedNote создаёт файл `name` с телом `body` в `dir` и регистрирует
// строку в notes с заданным content_hash. Возвращает абсолютный путь
// к созданному файлу.
func seedNote(t *testing.T, conn *sql.DB, dir, name, hash, body string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write file %s: %v", full, err)
	}
	_, err := conn.Exec(
		`INSERT INTO notes (path, mtime, size, content_hash, title, frontmatter) VALUES (?, ?, ?, ?, ?, ?)`,
		full, int64(1), int64(len(body)), hash, sql.NullString{}, sql.NullString{},
	)
	if err != nil {
		t.Fatalf("insert note %s: %v", full, err)
	}
	return full
}

func cfgFor(qdrantURL, embedURL string, batchSize int) config.EmbeddingConfig {
	return config.EmbeddingConfig{
		QdrantURL:  qdrantURL,
		Collection: "mdx",
		Models: []config.ModelConfig{{
			Name:         "m1",
			Endpoint:     embedURL,
			EndpointKind: "openai",
			APIModelName: "test",
			Dim:          2,
			Distance:     "cosine",
			BatchSize:    batchSize,
		}},
	}
}

// --- tests --------------------------------------------------------------

func TestRunEmbedFreshThenIdempotent(t *testing.T) {
	qd, qdState := newMockQdrant(t, "mdx")
	defer qd.Close()
	em, emState := newMockOpenAI(t)
	defer em.Close()

	conn := openMigratedDB(t)
	dir := t.TempDir()
	seedNote(t, conn, dir, "a.md", "ha", "alpha")
	seedNote(t, conn, dir, "b.md", "hb", "beta")
	seedNote(t, conn, dir, "c.md", "hc", "gamma")

	cfg := cfgFor(qd.URL, em.URL, 3)

	stats, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{})
	if err != nil {
		t.Fatalf("first RunEmbed: %v", err)
	}
	if stats.Embedded != 3 || stats.Skipped != 0 || stats.Failed != 0 {
		t.Errorf("first stats = %+v, want {Embedded:3 Skipped:0 Failed:0}", stats)
	}
	if !qdState.created {
		t.Error("collection was not created")
	}
	if got := qdState.pointCount(); got != 3 {
		t.Errorf("upserted points = %d, want 3", got)
	}
	if got := emState.callCount(); got != 1 {
		// 3 заметки при batch_size=3 → один embed-вызов.
		t.Errorf("embed calls = %d, want 1", got)
	}

	emRows := count(t, conn, `SELECT COUNT(*) FROM embeddings WHERE model = 'm1'`)
	if emRows != 3 {
		t.Errorf("embeddings rows = %d, want 3", emRows)
	}

	// Повторный прогон — никакой работы не должно случиться.
	qdState.reset()
	emCallsBefore := emState.callCount()

	stats, err = RunEmbed(context.Background(), conn, cfg, EmbedOptions{})
	if err != nil {
		t.Fatalf("second RunEmbed: %v", err)
	}
	if stats.Embedded != 0 || stats.Skipped != 3 || stats.Failed != 0 {
		t.Errorf("second stats = %+v, want {Embedded:0 Skipped:3 Failed:0}", stats)
	}
	if got := emState.callCount(); got != emCallsBefore {
		t.Errorf("idempotent run made %d new embed calls, want 0", got-emCallsBefore)
	}
	if got := qdState.pointCount(); got != 0 {
		t.Errorf("idempotent run upserted %d points, want 0", got)
	}
}

func TestRunEmbedContentHashChange(t *testing.T) {
	qd, qdState := newMockQdrant(t, "mdx")
	defer qd.Close()
	em, emState := newMockOpenAI(t)
	defer em.Close()

	conn := openMigratedDB(t)
	dir := t.TempDir()
	pathA := seedNote(t, conn, dir, "a.md", "h-a", "alpha")
	seedNote(t, conn, dir, "b.md", "h-b", "beta")

	cfg := cfgFor(qd.URL, em.URL, 8)

	if _, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{}); err != nil {
		t.Fatalf("priming RunEmbed: %v", err)
	}

	qdState.reset()
	emCallsBefore := emState.callCount()

	if _, err := conn.Exec(`UPDATE notes SET content_hash = ? WHERE path = ?`, "h-a-new", pathA); err != nil {
		t.Fatalf("update content_hash: %v", err)
	}

	stats, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{})
	if err != nil {
		t.Fatalf("RunEmbed after hash change: %v", err)
	}
	if stats.Embedded != 1 || stats.Skipped != 1 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want {Embedded:1 Skipped:1 Failed:0}", stats)
	}
	if delta := emState.callCount() - emCallsBefore; delta != 1 {
		t.Errorf("embed calls after hash change = %d, want 1", delta)
	}
	if got := qdState.pointCount(); got != 1 {
		t.Errorf("upserted points after hash change = %d, want 1", got)
	}

	var stored string
	if err := conn.QueryRow(
		`SELECT content_hash FROM embeddings WHERE path = ? AND model = 'm1'`, pathA,
	).Scan(&stored); err != nil {
		t.Fatalf("read embeddings.content_hash: %v", err)
	}
	if stored != "h-a-new" {
		t.Errorf("embeddings.content_hash = %q, want h-a-new", stored)
	}
}

func TestRunEmbedAllFlag(t *testing.T) {
	qd, qdState := newMockQdrant(t, "mdx")
	defer qd.Close()
	em, emState := newMockOpenAI(t)
	defer em.Close()

	conn := openMigratedDB(t)
	dir := t.TempDir()
	seedNote(t, conn, dir, "a.md", "ha", "alpha")
	seedNote(t, conn, dir, "b.md", "hb", "beta")

	cfg := cfgFor(qd.URL, em.URL, 8)

	if _, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{}); err != nil {
		t.Fatalf("priming RunEmbed: %v", err)
	}

	qdState.reset()
	emCallsBefore := emState.callCount()

	stats, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{All: true})
	if err != nil {
		t.Fatalf("RunEmbed --all: %v", err)
	}
	if stats.Embedded != 2 || stats.Skipped != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want {Embedded:2 Skipped:0 Failed:0}", stats)
	}
	if delta := emState.callCount() - emCallsBefore; delta != 1 {
		t.Errorf("embed calls under --all = %d, want 1", delta)
	}
	if got := qdState.pointCount(); got != 2 {
		t.Errorf("upserted points under --all = %d, want 2", got)
	}
}

func TestRunEmbedEmbedFailurePartial(t *testing.T) {
	qd, qdState := newMockQdrant(t, "mdx")
	defer qd.Close()
	em, emState := newMockOpenAI(t)
	defer em.Close()

	conn := openMigratedDB(t)
	dir := t.TempDir()
	seedNote(t, conn, dir, "a.md", "ha", "alpha")
	seedNote(t, conn, dir, "b.md", "hb", "beta")
	seedNote(t, conn, dir, "c.md", "hc", "gamma")

	// batch_size=1 → каждый запрос отдельный, отказ моделью одного
	// батча не задевает остальные.
	cfg := cfgFor(qd.URL, em.URL, 1)

	emState.mu.Lock()
	emState.failOnce = true
	emState.mu.Unlock()

	stats, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{})
	if err != nil {
		t.Fatalf("RunEmbed: want no error, got %v", err)
	}
	if stats.Failed != 1 {
		t.Errorf("Failed = %d, want 1", stats.Failed)
	}
	if stats.Embedded != 2 {
		t.Errorf("Embedded = %d, want 2", stats.Embedded)
	}
	if got := qdState.pointCount(); got != 2 {
		t.Errorf("upserted points = %d, want 2", got)
	}
}

func TestRunEmbedUnknownModel(t *testing.T) {
	qd, _ := newMockQdrant(t, "mdx")
	defer qd.Close()
	em, _ := newMockOpenAI(t)
	defer em.Close()

	conn := openMigratedDB(t)
	cfg := cfgFor(qd.URL, em.URL, 4)

	_, err := RunEmbed(context.Background(), conn, cfg, EmbedOptions{Model: "nope"})
	if err == nil {
		t.Fatal("RunEmbed: want error for unknown model")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("err = %q, want substring %q", err.Error(), "nope")
	}
}
