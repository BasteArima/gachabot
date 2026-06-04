package migrations

import (
	"database/sql"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"
)

// sqlFiles holds every *.sql migration, embedded into the binary at build time.
// Because they are embedded, the migrations travel with the compiled bot and run
// on startup — no need to mount SQL files into the container or run psql by hand.
//
//go:embed *.sql
var sqlFiles embed.FS

// Run applies every embedded migration that has not been applied yet, in
// filename order. Each migration runs inside its own transaction and is recorded
// in the schema_migrations table, so re-running Run is a no-op once everything is
// applied. Migrations should be written to be idempotent (e.g. ADD COLUMN IF NOT
// EXISTS) so a manual pre-apply followed by Run stays safe.
func Run(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	)`); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	entries, err := sqlFiles.ReadDir(".")
	if err != nil {
		return fmt.Errorf("failed to read embedded migrations: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	applied := 0
	for _, name := range names {
		var exists bool
		if err := db.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
		).Scan(&exists); err != nil {
			return fmt.Errorf("failed to check migration %s: %w", name, err)
		}
		if exists {
			continue
		}

		content, err := sqlFiles.ReadFile(name)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin tx for %s: %w", name, err)
		}
		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s failed: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", name, err)
		}

		log.Printf("Applied migration: %s", name)
		applied++
	}

	if applied == 0 {
		log.Println("Database schema is up to date, no migrations to apply")
	} else {
		log.Printf("Applied %d migration(s)", applied)
	}
	return nil
}
