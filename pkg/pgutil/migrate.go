package pgutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migrate ejecuta las migraciones pendientes del directorio dado.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	dsn := buildMigrateDSN(pool)

	m, err := migrate.New("file://"+migrationsDir, dsn)
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
func MigrateDown(pool *pgxpool.Pool, migrationsDir string, steps int) error {
	dsn := buildMigrateDSN(pool)

	m, err := migrate.New("file://"+migrationsDir, dsn)
	if err != nil {
		return fmt.Errorf("pgutil: init migrate (down) for %q: %w", migrationsDir, err)
	}
	defer m.Close()

	if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("pgutil: migrate down %d steps: %w", steps, err)
	}
	return nil
}

// buildMigrateDSN construye el DSN en formato pgx5:// para golang-migrate
// usando los campos individuales de la config del pool — evita el problema
// de ConnString() que devuelve formato libpq key=value incompatible con migrate.
func buildMigrateDSN(pool *pgxpool.Pool) string {
	cfg := pool.Config().ConnConfig

	dsn := fmt.Sprintf("pgx5://%s:%s@%s:%d/%s",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
	)

	if cfg.TLSConfig == nil {
		dsn += "?sslmode=disable"
	}

	return dsn
}

// normalizeDSN convierte postgresql:// o postgres:// a pgx5://
func normalizeDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	switch {
	case strings.HasPrefix(dsn, "postgresql://"):
		return "pgx5://" + dsn[len("postgresql://"):]
	case strings.HasPrefix(dsn, "postgres://"):
		return "pgx5://" + dsn[len("postgres://"):]
	case strings.HasPrefix(dsn, "pgx5://"):
		return dsn
	default:
		return "pgx5://" + dsn
	}
}
