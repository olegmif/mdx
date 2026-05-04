package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/olegmif/mdx/lsp/internal/config"
)

// distanceCosine — единственное допустимое значение метрики на M0
// (валидируется в config.LoadEmbedding). Передаётся в Qdrant дословно.
const distanceCosine = "Cosine"

// QdrantClient — REST-клиент для одной инсталляции Qdrant.
// Таймаут берётся из общего defaultHTTPTimeout пакета embed.
type QdrantClient struct {
	baseURL string
	http    *http.Client
}

func NewQdrantClient(baseURL string) *QdrantClient {
	return &QdrantClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// vectorParams — описание одного именованного вектора в коллекции.
type vectorParams struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

type createCollectionRequest struct {
	Vectors map[string]vectorParams `json:"vectors"`
}

type updateCollectionRequest struct {
	Vectors map[string]vectorParams `json:"vectors"`
}

// EnsureCollection приводит коллекцию в соответствие с cfg.Models:
// создаёт при отсутствии либо дополняет недостающими именованными
// векторами. Существующие векторы не трогает — даже если у модели в
// конфиге сменилась размерность, старый вектор не пересоздаётся.
// Сценарий «перенастройка существующего named vector» сознательно
// вне скоупа M0: Qdrant требует пересоздания коллекции, что лучше
// решать через отдельную операцию.
func (q *QdrantClient) EnsureCollection(ctx context.Context, cfg config.EmbeddingConfig) error {
	path := "/collections/" + cfg.Collection
	op := fmt.Sprintf("GET collection %q", cfg.Collection)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, q.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("qdrant: build %s: %w", op, err)
	}
	resp, err := q.http.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant: %s: %w", op, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return q.createCollection(ctx, cfg)
	case http.StatusOK:
		var info struct {
			Result struct {
				Config struct {
					Params struct {
						Vectors map[string]json.RawMessage `json:"vectors"`
					} `json:"params"`
				} `json:"config"`
			} `json:"result"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			return fmt.Errorf("qdrant: decode %s: %w", op, err)
		}
		existing := info.Result.Config.Params.Vectors
		missing := make(map[string]vectorParams)
		for _, m := range cfg.Models {
			if _, ok := existing[m.Name]; !ok {
				missing[m.Name] = vectorParams{Size: m.Dim, Distance: distanceCosine}
			}
		}
		if len(missing) == 0 {
			return nil
		}
		return q.patchCollection(ctx, cfg.Collection, missing)
	default:
		return q.statusError(resp, op)
	}
}

func (q *QdrantClient) createCollection(ctx context.Context, cfg config.EmbeddingConfig) error {
	vectors := make(map[string]vectorParams, len(cfg.Models))
	for _, m := range cfg.Models {
		vectors[m.Name] = vectorParams{Size: m.Dim, Distance: distanceCosine}
	}
	body, err := json.Marshal(createCollectionRequest{Vectors: vectors})
	if err != nil {
		return fmt.Errorf("qdrant: marshal create collection %q: %w", cfg.Collection, err)
	}
	return q.doJSON(ctx, http.MethodPut, "/collections/"+cfg.Collection, body,
		fmt.Sprintf("create collection %q", cfg.Collection))
}

func (q *QdrantClient) patchCollection(ctx context.Context, name string, vectors map[string]vectorParams) error {
	body, err := json.Marshal(updateCollectionRequest{Vectors: vectors})
	if err != nil {
		return fmt.Errorf("qdrant: marshal patch collection %q: %w", name, err)
	}
	return q.doJSON(ctx, http.MethodPatch, "/collections/"+name, body,
		fmt.Sprintf("patch collection %q", name))
}

// Point — одна точка в Qdrant. Vectors — карта name модели → вектор;
// Qdrant хранит named vectors per-point параллельно.
type Point struct {
	ID      uuid.UUID
	Vectors map[string][]float32
	Payload map[string]any
}

type upsertRequest struct {
	Points []upsertPoint `json:"points"`
}

type upsertPoint struct {
	ID      string               `json:"id"`
	Vector  map[string][]float32 `json:"vector"`
	Payload map[string]any       `json:"payload,omitempty"`
}

// Upsert загружает batch точек одним запросом, ждёт подтверждения
// записи (`wait=true`). Пустой batch — no-op без обращения к серверу.
func (q *QdrantClient) Upsert(ctx context.Context, collection string, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	out := make([]upsertPoint, len(points))
	for i, p := range points {
		out[i] = upsertPoint{
			ID:      p.ID.String(),
			Vector:  p.Vectors,
			Payload: p.Payload,
		}
	}
	body, err := json.Marshal(upsertRequest{Points: out})
	if err != nil {
		return fmt.Errorf("qdrant: marshal upsert %q: %w", collection, err)
	}
	return q.doJSON(ctx, http.MethodPut,
		"/collections/"+collection+"/points?wait=true", body,
		fmt.Sprintf("upsert into %q", collection))
}

// doJSON отправляет body как JSON, проверяет 2xx и опциональный
// `status: "ok"` в ответе. Тело ответа в успешном случае не возвращается:
// все вызовы M0 либо ack-only (создание/upsert), либо имеют отдельный
// путь чтения (EnsureCollection делает GET сама).
func (q *QdrantClient) doJSON(ctx context.Context, method, path string, body []byte, op string) error {
	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("qdrant: build %s: %w", op, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := q.http.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant: %s: %w", op, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return q.statusError(resp, op)
	}

	var ack struct {
		Status string `json:"status"`
	}
	// Декод опционален: некоторые ответы Qdrant пустые. Любая ошибка
	// декодирования трактуется как «ack нет», и это не ошибка операции.
	if err := json.NewDecoder(resp.Body).Decode(&ack); err == nil &&
		ack.Status != "" && ack.Status != "ok" {
		return fmt.Errorf("qdrant: %s: status=%s", op, ack.Status)
	}
	return nil
}

func (q *QdrantClient) statusError(resp *http.Response, op string) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("qdrant: %s: http %d: %s",
		op, resp.StatusCode, firstLine(snippet))
}
