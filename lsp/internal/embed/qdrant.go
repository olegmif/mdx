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

// SearchHit — одна точка, возвращённая Qdrant в ответ на k-NN-поиск.
// Payload приходит как произвольный объект; вызывающий код вытаскивает
// нужные поля сам.
type SearchHit struct {
	ID      string
	Score   float32
	Payload map[string]any
}

type searchRequest struct {
	Vector      searchVector `json:"vector"`
	Limit       int          `json:"limit"`
	WithPayload bool         `json:"with_payload"`
}

type searchVector struct {
	Name   string    `json:"name"`
	Vector []float32 `json:"vector"`
}

type searchResponse struct {
	Result []searchPoint `json:"result"`
}

type searchPoint struct {
	ID      any            `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

// Search выполняет k-NN-поиск по named vector vectorName в коллекции
// collection и возвращает не более limit точек с payload. Без фильтров.
// Сортировку Qdrant делает на своей стороне; клиент порядок не меняет.
func (q *QdrantClient) Search(ctx context.Context, collection, vectorName string, vector []float32, limit int) ([]SearchHit, error) {
	body, err := json.Marshal(searchRequest{
		Vector:      searchVector{Name: vectorName, Vector: vector},
		Limit:       limit,
		WithPayload: true,
	})
	if err != nil {
		return nil, fmt.Errorf("qdrant: marshal search %q: %w", collection, err)
	}
	op := fmt.Sprintf("search %q", collection)
	var resp searchResponse
	if err := q.doJSONReply(ctx, http.MethodPost,
		"/collections/"+collection+"/points/search", body, op, &resp); err != nil {
		return nil, err
	}
	hits := make([]SearchHit, len(resp.Result))
	for i, r := range resp.Result {
		// id у Qdrant может быть UUID-строкой или uint64; на M0–M1 наши
		// id всегда UUID, но защищаемся от регрессии типа.
		hits[i] = SearchHit{
			ID:      fmt.Sprintf("%v", r.ID),
			Score:   r.Score,
			Payload: r.Payload,
		}
	}
	return hits, nil
}

// ScrollPoint — одна точка в выдаче Scroll. Payload сужен до того,
// что нужно gc'ю (только path); полный payload остаётся в коллекции
// нетронутым.
type ScrollPoint struct {
	ID   string
	Path string
}

const defaultScrollBatch = 1000

type scrollRequest struct {
	Limit       int      `json:"limit"`
	WithPayload []string `json:"with_payload"`
	WithVector  bool     `json:"with_vector"`
	Offset      any      `json:"offset,omitempty"`
}

type scrollResponse struct {
	Result struct {
		Points         []scrollResponsePoint `json:"points"`
		NextPageOffset any                   `json:"next_page_offset"`
	} `json:"result"`
}

type scrollResponsePoint struct {
	ID      any            `json:"id"`
	Payload map[string]any `json:"payload"`
}

// Scroll итерирует все точки коллекции и возвращает их id и payload.path.
// Внутри делает несколько HTTP-запросов с pagination через
// next_page_offset; завершается, когда сервер возвращает null. batchSize
// задаёт размер одной страницы; <=0 трактуется как defaultScrollBatch.
func (q *QdrantClient) Scroll(ctx context.Context, collection string, batchSize int) ([]ScrollPoint, error) {
	if batchSize <= 0 {
		batchSize = defaultScrollBatch
	}
	op := fmt.Sprintf("scroll %q", collection)
	path := "/collections/" + collection + "/points/scroll"

	var out []ScrollPoint
	var offset any
	for {
		body, err := json.Marshal(scrollRequest{
			Limit:       batchSize,
			WithPayload: []string{"path"},
			WithVector:  false,
			Offset:      offset,
		})
		if err != nil {
			return nil, fmt.Errorf("qdrant: marshal %s: %w", op, err)
		}
		var resp scrollResponse
		if err := q.doJSONReply(ctx, http.MethodPost, path, body, op, &resp); err != nil {
			return nil, err
		}
		for _, p := range resp.Result.Points {
			sp := ScrollPoint{ID: fmt.Sprintf("%v", p.ID)}
			if v, ok := p.Payload["path"]; ok {
				if s, ok := v.(string); ok {
					sp.Path = s
				}
			}
			out = append(out, sp)
		}
		if resp.Result.NextPageOffset == nil {
			return out, nil
		}
		offset = resp.Result.NextPageOffset
	}
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

// doJSONReply отправляет body как JSON и декодирует тело успешного
// ответа в out. Используется операциями, которым нужно прочитать
// результат (Search). Ack-only вызовы (createCollection, patchCollection,
// Upsert) ходят через doJSON, чтобы не таскать лишний параметр.
func (q *QdrantClient) doJSONReply(ctx context.Context, method, path string, body []byte, op string, out any) error {
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
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("qdrant: %s: decode: %w", op, err)
	}
	return nil
}

func (q *QdrantClient) statusError(resp *http.Response, op string) error {
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("qdrant: %s: http %d: %s",
		op, resp.StatusCode, firstLine(snippet))
}
