package db

import (
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

const currentVersion = 1

// Migrate ensures the database schema is at currentVersion.
// On a fresh DB it applies schema.sql and records version=1.
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
	if have >= currentVersion {
		return nil
	}
	return fmt.Errorf("unsupported migration from v%d to v%d", have, currentVersion)
}
