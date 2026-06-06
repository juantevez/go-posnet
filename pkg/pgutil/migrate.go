package pgutil

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate es un no-op en este entorno — las tablas son creadas por el
// init script de Postgres en docker-entrypoint-initdb.d/01-init.sql.
// golang-migrate está deshabilitado para el MVP.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	slog.Info("migrations skipped — tables created by postgres init script",
		slog.String("dir", migrationsDir),
	)
	return nil
}

// MigrateDown es un no-op en este entorno.
func MigrateDown(pool *pgxpool.Pool, migrationsDir string, steps int) error {
	return nil
}
