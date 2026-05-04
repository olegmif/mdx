package cli

import (
	"context"
	"database/sql"
	"errors"
	"time"
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

// RunEmbed is filled in by Step 8. The signature widens to accept
// config.EmbeddingConfig in Step 2.
func RunEmbed(ctx context.Context, conn *sql.DB, opts EmbedOptions) (EmbedStats, error) {
	return EmbedStats{}, errors.New("embed: not implemented")
}
