package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/olegmif/mdx/lsp/internal/config"
	"github.com/olegmif/mdx/lsp/internal/db"
	"github.com/olegmif/mdx/lsp/internal/embed"
)

// EmbedOptions collects flags driving a single mdx embed run.
type EmbedOptions struct {
	Model string // empty = all models from config
	All   bool   // ignore embeddings table and recompute every note
}

// EmbedStats summarizes one mdx embed run.
type EmbedStats struct {
	Embedded int
	Skipped  int
	Failed   int
	Elapsed  time.Duration
}

// RunEmbed orchestrates one embedding pass:
//
//   - resolves which models to use (one or all),
//   - reconciles the Qdrant collection with cfg.Models (creates/patches),
//   - for each model picks notes that need (re-)embedding from db,
//     reads file bodies, calls the embedding API in batches, upserts the
//     resulting points into Qdrant, then records the embedding in db.
//
// Per-batch errors are reported to stderr and counted; they do not abort
// the run. Context cancellation aborts the loop and returns ctx.Err()
// together with whatever stats have accumulated.
func RunEmbed(ctx context.Context, conn *sql.DB, cfg config.EmbeddingConfig, opts EmbedOptions) (EmbedStats, error) {
	startedAt := time.Now()
	var stats EmbedStats

	models, err := selectModels(cfg, opts.Model)
	if err != nil {
		stats.Elapsed = time.Since(startedAt)
		return stats, err
	}

	qd := embed.NewQdrantClient(cfg.QdrantURL)
	// Коллекция держит схему всего конфига даже когда --model сужает
	// прогон до одной модели — иначе между запусками было бы «то один
	// named vector есть, то нет».
	if err := qd.EnsureCollection(ctx, cfg); err != nil {
		stats.Elapsed = time.Since(startedAt)
		return stats, err
	}

	var totalNotes int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes`).Scan(&totalNotes); err != nil {
		stats.Elapsed = time.Since(startedAt)
		return stats, fmt.Errorf("count notes: %w", err)
	}

	for _, model := range models {
		if err := ctx.Err(); err != nil {
			stats.Elapsed = time.Since(startedAt)
			return stats, err
		}

		pending, err := db.PendingEmbeddings(ctx, conn, model.Name, opts.All)
		if err != nil {
			stats.Elapsed = time.Since(startedAt)
			return stats, err
		}
		// Заметки, у которых вектор уже актуален — не идут в pending.
		// Их и считаем «пропущенными» для этой модели. При opts.All=true
		// pending содержит все заметки, и Skipped по этой модели = 0.
		stats.Skipped += totalNotes - len(pending)

		if len(pending) == 0 {
			continue
		}

		client := embed.NewModelClient(model)
		for i := 0; i < len(pending); i += model.BatchSize {
			if err := ctx.Err(); err != nil {
				stats.Elapsed = time.Since(startedAt)
				return stats, err
			}
			end := i + model.BatchSize
			if end > len(pending) {
				end = len(pending)
			}
			processEmbedBatch(ctx, conn, client, qd, cfg.Collection, model, pending[i:end], &stats)
		}
	}

	stats.Elapsed = time.Since(startedAt)
	return stats, nil
}

// selectModels возвращает либо все модели из конфига, либо одну
// по имени; при отсутствии указанной модели возвращает ошибку.
func selectModels(cfg config.EmbeddingConfig, name string) ([]config.ModelConfig, error) {
	if name == "" {
		return cfg.Models, nil
	}
	for _, m := range cfg.Models {
		if m.Name == name {
			return []config.ModelConfig{m}, nil
		}
	}
	return nil, fmt.Errorf("model %q not found in embedding config", name)
}

// processEmbedBatch обрабатывает один batch заметок для одной модели:
// читает файлы, считает embeddings, делает upsert в Qdrant, записывает
// результат в таблицу embeddings. Ошибки на любом этапе логируются и
// учитываются в stats.Failed; цикл продолжается со следующего батча.
func processEmbedBatch(
	ctx context.Context,
	conn *sql.DB,
	client *embed.ModelClient,
	qd *embed.QdrantClient,
	collection string,
	model config.ModelConfig,
	batch []db.PendingNote,
	stats *EmbedStats,
) {
	// 1. Читаем содержимое; нечитаемые файлы выпадают из батча, но
	//    остальные продолжают обрабатываться.
	valid := make([]db.PendingNote, 0, len(batch))
	texts := make([]string, 0, len(batch))
	for _, p := range batch {
		data, err := os.ReadFile(p.Path)
		if err != nil {
			stats.Failed++
			fmt.Fprintf(os.Stderr, "mdx: embed %s/%s: read: %v\n", model.Name, p.Path, err)
			continue
		}
		valid = append(valid, p)
		texts = append(texts, string(data))
	}
	if len(valid) == 0 {
		return
	}

	// 2. Векторизуем батч моделью.
	vectors, err := client.Embed(ctx, texts, model.DocumentPrefix)
	if err != nil {
		stats.Failed += len(valid)
		fmt.Fprintf(os.Stderr, "mdx: embed model %s: %v\n", model.Name, err)
		return
	}
	if len(vectors) != len(valid) {
		stats.Failed += len(valid)
		fmt.Fprintf(os.Stderr, "mdx: embed model %s: got %d vectors for %d inputs\n",
			model.Name, len(vectors), len(valid))
		return
	}

	// 3. Upsert в Qdrant. Точки одного батча идут одним запросом.
	points := make([]embed.Point, len(valid))
	for i, p := range valid {
		points[i] = embed.Point{
			ID:      embed.PointID(p.Path),
			Vectors: map[string][]float32{model.Name: vectors[i]},
			Payload: payloadFor(p),
		}
	}
	if err := qd.Upsert(ctx, collection, points); err != nil {
		stats.Failed += len(valid)
		fmt.Fprintf(os.Stderr, "mdx: qdrant upsert %s: %v\n", model.Name, err)
		return
	}

	// 4. Фиксируем факт embed-а в БД. Точка в Qdrant к этому моменту
	//    уже записана, так что сбой record'а — это рассинхрон, который
	//    при следующем прогоне починится сам через сравнение content_hash.
	now := time.Now().Unix()
	for _, p := range valid {
		if err := db.RecordEmbedding(ctx, conn, p.Path, model.Name, p.ContentHash, now); err != nil {
			stats.Failed++
			fmt.Fprintf(os.Stderr, "mdx: record %s/%s: %v\n", model.Name, p.Path, err)
			continue
		}
		stats.Embedded++
	}
}

// payloadFor собирает payload для точки Qdrant из строки notes.
// Title/Frontmatter попадают в payload только если они не NULL —
// иначе ключи отсутствуют, чтобы не путать nil со строкой "null".
func payloadFor(p db.PendingNote) map[string]any {
	payload := map[string]any{
		"path":         p.Path,
		"mtime":        p.Mtime,
		"content_hash": p.ContentHash,
	}
	if p.Title.Valid {
		payload["title"] = p.Title.String
	}
	if p.Frontmatter.Valid {
		payload["frontmatter"] = p.Frontmatter.String
	}
	return payload
}
