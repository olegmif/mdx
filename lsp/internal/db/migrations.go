package db

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

const currentVersion = 2

// Migrate ensures the database schema is at currentVersion.
// On a fresh DB it applies schema.sql and records the current version
// as a single row. On an existing DB it applies pending migrations one
// at a time, recording each version as a separate row.
func Migrate(conn *sql.DB) error {
	var name string
	err := conn.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='schema_version'`,
	).Scan(&name)

	switch err {
	case sql.ErrNoRows:
		return applyFreshSchema(conn)
	case nil:
		return upgradeSchema(conn)
	default:
		return fmt.Errorf("probe schema_version: %w", err)
	}
}

func applyFreshSchema(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO schema_version (version) VALUES (?)`,
		currentVersion,
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

func upgradeSchema(conn *sql.DB) error {
	var have int
	if err := conn.QueryRow(
		`SELECT COALESCE(MAX(version), 0) FROM schema_version`,
	).Scan(&have); err != nil {
		return fmt.Errorf("read version: %w", err)
	}
	for v := have + 1; v <= currentVersion; v++ {
		if err := applyMigration(conn, v); err != nil {
			return fmt.Errorf("migrate to v%d: %w", v, err)
		}
	}
	return nil
}

func applyMigration(conn *sql.DB, version int) error {
	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	switch version {
	case 2:
		if _, err := tx.Exec(migrationV2); err != nil {
			return fmt.Errorf("apply: %w", err)
		}
	default:
		return fmt.Errorf("no migration registered for v%d", version)
	}

	if _, err := tx.Exec(
		`INSERT INTO schema_version (version) VALUES (?)`,
		version,
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return tx.Commit()
}

// migrationV2 mirrors the embeddings block in schema.sql. The duplication
// is intentional: schema.sql installs the full current schema in one shot
// for fresh databases, while migrationV2 incrementally upgrades existing
// v1 databases.
const migrationV2 = `
CREATE TABLE embeddings (
  path          TEXT NOT NULL,
  model         TEXT NOT NULL,
  content_hash  TEXT NOT NULL,
  embedded_at   INTEGER NOT NULL,
  PRIMARY KEY (path, model),
  FOREIGN KEY (path) REFERENCES notes(path) ON DELETE CASCADE
);
CREATE INDEX idx_embeddings_model ON embeddings(model);
`
