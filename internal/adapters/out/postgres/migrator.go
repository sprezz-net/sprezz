package postgres

import (
	"context"
	"embed"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// RunDatabaseMigrations executes outstanding SQL schema updates using embedded files.
// This operation MUST be invoked exclusively by the central server on boot.
func RunDatabaseMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	log.Println("Initializing embedded database schema migration audit...")

	// 1. Open a temporary standard sql.DB connection from the existing high-performance pgxpool
	db := stdlib.OpenDBFromPool(pool)

	// 2. Configure goose parameters to target the embedded memory file-system tree
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect(string(goose.DialectPostgres)); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed configuring migration database dialect schema: %w", err)
	}

	// 3. Apply all outstanding structural modifications forward atomically inside a transaction scope
	// Goose programmatically tracks historical iterations inside the internal table: "goose_db_version"
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		_ = db.Close()
		return fmt.Errorf("database migration execution process aborted: %w", err)
	}

	// 4. Detach standard bridge handle carefully to return pooling resources to repository layers
	if err := db.Close(); err != nil {
		return fmt.Errorf("failed to cleanly close transient standard driver adapter link: %w", err)
	}

	log.Println("Database migration sequence completed successfully. Schema is fully synchronized.")
	return nil
}
