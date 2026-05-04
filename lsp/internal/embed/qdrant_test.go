package embed

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/olegmif/mdx/lsp/internal/config"
)

func TestEnsureCollectionCreatesWhenAbsent(t *testing.T) {
	var seen createCollectionRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			http.NotFound(w, r)
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
				t.Fatalf("decode put: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":true,"status":"ok"}`)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	cfg := config.EmbeddingConfig{
		QdrantURL:  srv.URL,
		Collection: "mdx",
		Models: []config.ModelConfig{
			{Name: "m1", Dim: 8},
			{Name: "m2", Dim: 16},
		},
	}
	c := NewQdrantClient(srv.URL)
	if err := c.EnsureCollection(context.Background(), cfg); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if got, want := seen.Vectors["m1"], (vectorParams{Size: 8, Distance: distanceCosine}); got != want {
		t.Errorf("m1 = %+v, want %+v", got, want)
	}
	if got, want := seen.Vectors["m2"], (vectorParams{Size: 16, Distance: distanceCosine}); got != want {
		t.Errorf("m2 = %+v, want %+v", got, want)
	}
}

func TestEnsureCollectionPatchesMissing(t *testing.T) {
	var seenPatch updateCollectionRequest
	var sawPut bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"result": {"config": {"params": {"vectors": {"m1": {"size": 8, "distance": "Cosine"}}}}},
				"status": "ok"
			}`)
		case http.MethodPatch:
			if err := json.NewDecoder(r.Body).Decode(&seenPatch); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"result":true,"status":"ok"}`)
		case http.MethodPut:
			sawPut = true
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer srv.Close()

	cfg := config.EmbeddingConfig{
		QdrantURL:  srv.URL,
		Collection: "mdx",
		Models: []config.ModelConfig{
			{Name: "m1", Dim: 8},
			{Name: "m2", Dim: 16},
		},
	}
	c := NewQdrantClient(srv.URL)
	if err := c.EnsureCollection(context.Background(), cfg); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if sawPut {
		t.Error("PUT observed; expected only GET+PATCH when collection exists")
	}
	if _, ok := seenPatch.Vectors["m1"]; ok {
		t.Error("PATCH includes m1 (already exists in collection)")
	}
	if got, want := seenPatch.Vectors["m2"], (vectorParams{Size: 16, Distance: distanceCosine}); got != want {
		t.Errorf("m2 in patch = %+v, want %+v", got, want)
	}
}

func TestEnsureCollectionAllVectorsPresent(t *testing.T) {
	var sawPatch, sawPut bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{
				"result": {"config": {"params": {"vectors": {
					"m1": {"size": 8,  "distance": "Cosine"},
					"m2": {"size": 16, "distance": "Cosine"}
				}}}},
				"status": "ok"
			}`)
		case http.MethodPatch:
			sawPatch = true
		case http.MethodPut:
			sawPut = true
		}
	}))
	defer srv.Close()

	cfg := config.EmbeddingConfig{
		QdrantURL:  srv.URL,
		Collection: "mdx",
		Models: []config.ModelConfig{
			{Name: "m1", Dim: 8},
			{Name: "m2", Dim: 16},
		},
	}
	if err := NewQdrantClient(srv.URL).EnsureCollection(context.Background(), cfg); err != nil {
		t.Fatalf("EnsureCollection: %v", err)
	}
	if sawPatch || sawPut {
		t.Errorf("unexpected write: patch=%v put=%v", sawPatch, sawPut)
	}
}

func TestEnsureCollectionGETError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := config.EmbeddingConfig{
		QdrantURL:  srv.URL,
		Collection: "mdx",
		Models:     []config.ModelConfig{{Name: "m1", Dim: 8}},
	}
	err := NewQdrantClient(srv.URL).EnsureCollection(context.Background(), cfg)
	if err == nil {
		t.Fatal("EnsureCollection: want error")
	}
	if !strings.Contains(err.Error(), "mdx") || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want substrings [mdx, 500]", err.Error())
	}
}

func TestQdrantUpsert(t *testing.T) {
	var seenURL *url.URL
	var seenBody upsertRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"result":{},"status":"ok"}`)
	}))
	defer srv.Close()

	c := NewQdrantClient(srv.URL)
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	points := []Point{{
		ID:      id,
		Vectors: map[string][]float32{"m1": {1, 2, 3}},
		Payload: map[string]any{"path": "/n.md"},
	}}
	if err := c.Upsert(context.Background(), "mdx", points); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if seenURL.Path != "/collections/mdx/points" {
		t.Errorf("path = %s, want /collections/mdx/points", seenURL.Path)
	}
	if got := seenURL.Query().Get("wait"); got != "true" {
		t.Errorf("wait = %q, want true", got)
	}
	if len(seenBody.Points) != 1 {
		t.Fatalf("points = %d, want 1", len(seenBody.Points))
	}
	p := seenBody.Points[0]
	if p.ID != id.String() {
		t.Errorf("id = %s, want %s", p.ID, id)
	}
	if got := p.Vector["m1"]; len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Errorf("vector m1 = %v, want [1 2 3]", got)
	}
	if p.Payload["path"] != "/n.md" {
		t.Errorf("payload.path = %v, want /n.md", p.Payload["path"])
	}
}

func TestQdrantUpsertEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Upsert with empty points list should not call the server")
	}))
	defer srv.Close()

	if err := NewQdrantClient(srv.URL).Upsert(context.Background(), "mdx", nil); err != nil {
		t.Fatalf("Upsert(nil): %v", err)
	}
}

func TestQdrantSearch(t *testing.T) {
	var seenURL *url.URL
	var seenBody searchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURL = r.URL
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"result": [
				{"id": "00000000-0000-0000-0000-000000000001", "score": 0.9, "payload": {"path": "/n/a.md", "title": "A"}},
				{"id": "00000000-0000-0000-0000-000000000002", "score": 0.7, "payload": {"path": "/n/b.md"}}
			],
			"status": "ok"
		}`)
	}))
	defer srv.Close()

	c := NewQdrantClient(srv.URL)
	hits, err := c.Search(context.Background(), "mdx", "m1", []float32{0.1, 0.2}, 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if seenURL.Path != "/collections/mdx/points/search" {
		t.Errorf("path = %s, want /collections/mdx/points/search", seenURL.Path)
	}
	if seenBody.Vector.Name != "m1" {
		t.Errorf("vector.name = %q, want m1", seenBody.Vector.Name)
	}
	if got := seenBody.Vector.Vector; len(got) != 2 || got[0] != 0.1 || got[1] != 0.2 {
		t.Errorf("vector.vector = %v, want [0.1 0.2]", got)
	}
	if seenBody.Limit != 5 {
		t.Errorf("limit = %d, want 5", seenBody.Limit)
	}
	if !seenBody.WithPayload {
		t.Error("with_payload = false, want true")
	}

	if len(hits) != 2 {
		t.Fatalf("hits = %d, want 2", len(hits))
	}
	if hits[0].ID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("hits[0].ID = %s", hits[0].ID)
	}
	if hits[0].Score != 0.9 {
		t.Errorf("hits[0].Score = %v, want 0.9", hits[0].Score)
	}
	if hits[0].Payload["path"] != "/n/a.md" {
		t.Errorf("hits[0].Payload[path] = %v", hits[0].Payload["path"])
	}
	if hits[0].Payload["title"] != "A" {
		t.Errorf("hits[0].Payload[title] = %v", hits[0].Payload["title"])
	}
	if _, ok := hits[1].Payload["title"]; ok {
		t.Errorf("hits[1] has title in payload, want absent")
	}
}

func TestQdrantScrollPaginates(t *testing.T) {
	var seenURLs []*url.URL
	var seenBodies []scrollRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenURLs = append(seenURLs, r.URL)
		var body scrollRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode scroll: %v", err)
		}
		seenBodies = append(seenBodies, body)
		w.Header().Set("Content-Type", "application/json")
		switch body.Offset {
		case nil:
			_, _ = io.WriteString(w, `{
				"result": {
					"points": [
						{"id": "00000000-0000-0000-0000-000000000001", "payload": {"path": "/n/a.md"}},
						{"id": "00000000-0000-0000-0000-000000000002", "payload": {"path": "/n/b.md"}}
					],
					"next_page_offset": "p2"
				},
				"status": "ok"
			}`)
		case "p2":
			_, _ = io.WriteString(w, `{
				"result": {
					"points": [
						{"id": "00000000-0000-0000-0000-000000000003", "payload": {"path": "/n/c.md"}}
					],
					"next_page_offset": null
				},
				"status": "ok"
			}`)
		default:
			t.Fatalf("unexpected offset %#v", body.Offset)
		}
	}))
	defer srv.Close()

	pts, err := NewQdrantClient(srv.URL).Scroll(context.Background(), "mdx", 1000)
	if err != nil {
		t.Fatalf("Scroll: %v", err)
	}
	if len(seenURLs) != 2 {
		t.Fatalf("HTTP requests = %d, want 2", len(seenURLs))
	}
	if seenURLs[0].Path != "/collections/mdx/points/scroll" {
		t.Errorf("path = %s, want /collections/mdx/points/scroll", seenURLs[0].Path)
	}
	if seenBodies[0].Limit != 1000 {
		t.Errorf("limit = %d, want 1000", seenBodies[0].Limit)
	}
	if got := seenBodies[0].WithPayload; len(got) != 1 || got[0] != "path" {
		t.Errorf("with_payload = %v, want [path]", got)
	}
	if seenBodies[0].WithVector {
		t.Error("with_vector = true, want false")
	}
	if seenBodies[0].Offset != nil {
		t.Errorf("first request offset = %#v, want nil", seenBodies[0].Offset)
	}
	if seenBodies[1].Offset != "p2" {
		t.Errorf("second request offset = %#v, want p2", seenBodies[1].Offset)
	}
	if len(pts) != 3 {
		t.Fatalf("points = %d, want 3", len(pts))
	}
	want := []ScrollPoint{
		{ID: "00000000-0000-0000-0000-000000000001", Path: "/n/a.md"},
		{ID: "00000000-0000-0000-0000-000000000002", Path: "/n/b.md"},
		{ID: "00000000-0000-0000-0000-000000000003", Path: "/n/c.md"},
	}
	for i, w := range want {
		if pts[i] != w {
			t.Errorf("point %d = %+v, want %+v", i, pts[i], w)
		}
	}
}

func TestQdrantScrollPointWithoutPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"result": {
				"points": [
					{"id": "00000000-0000-0000-0000-0000000000aa", "payload": {}},
					{"id": "00000000-0000-0000-0000-0000000000bb", "payload": {"path": "/ok.md"}}
				],
				"next_page_offset": null
			},
			"status": "ok"
		}`)
	}))
	defer srv.Close()

	pts, err := NewQdrantClient(srv.URL).Scroll(context.Background(), "mdx", 0)
	if err != nil {
		t.Fatalf("Scroll: %v", err)
	}
	if len(pts) != 2 {
		t.Fatalf("points = %d, want 2", len(pts))
	}
	if pts[0].Path != "" {
		t.Errorf("pts[0].Path = %q, want empty", pts[0].Path)
	}
	if pts[1].Path != "/ok.md" {
		t.Errorf("pts[1].Path = %q, want /ok.md", pts[1].Path)
	}
}

func TestQdrantScrollHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewQdrantClient(srv.URL).Scroll(context.Background(), "mdx", 1000)
	if err == nil {
		t.Fatal("Scroll: want error")
	}
	if !strings.Contains(err.Error(), "mdx") || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want substrings [mdx, 500]", err.Error())
	}
}

func TestQdrantSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewQdrantClient(srv.URL).Search(context.Background(), "mdx", "m1", []float32{0.1}, 5)
	if err == nil {
		t.Fatal("Search: want error")
	}
	if !strings.Contains(err.Error(), "mdx") || !strings.Contains(err.Error(), "500") {
		t.Errorf("err = %q, want substrings [mdx, 500]", err.Error())
	}
}

func TestQdrantUpsertHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	err := NewQdrantClient(srv.URL).Upsert(context.Background(), "mdx", []Point{{
		ID:      uuid.New(),
		Vectors: map[string][]float32{"m1": {0}},
	}})
	if err == nil {
		t.Fatal("Upsert: want error")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "mdx") {
		t.Errorf("err = %q, want substrings [mdx, 502]", err.Error())
	}
}
