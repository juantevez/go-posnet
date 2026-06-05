package pgutil

import (
	"context"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate ejecuta las migraciones pendientes del directorio dado.
// Llamado en cmd/{bc}/main.go al arrancar el servicio, antes de servir tráfico.
// Usa golang-migrate con locking distribuido (advisory lock en Postgres).
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	dsn := pool.Config().ConnConfig.ConnString()

	m, err := migrate.New("file://"+migrationsDir, "pgx5://"+dsn)
	if err != nil {
		return fmt.Errorf("pgutil: init migrate for %q: %w", migrationsDir, err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("pgutil: run migrations from %q: %w", migrationsDir, err)
	}

	return nil
}

// MigrateDown revierte N migraciones hacia atrás.
// Solo para uso en tests de integración — nunca en producción directamente.
func MigrateDown(pool *pgxpool.Pool, migrationsDir string, steps int) error {
	dsn := pool.Config().ConnConfig.ConnString()

	m, err := migrate.New("file://"+migrationsDir, "pgx5://"+dsn)
	if err != nil {
		return fmt.Errorf("pgutil: init migrate (down) for %q: %w", migrationsDir, err)
	}
	defer m.Close()

	if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("pgutil: migrate down %d steps: %w", steps, err)
	}
	return nil
}
