package pgutil

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config contiene los parámetros del pool de conexiones.
type Config struct {
	DSN             string        // postgresql://user:pass@host:5432/db?sslmode=require
	MaxConns        int32         // Máximo de conexiones en el pool (recomendado: 10–25 por servicio)
	MinConns        int32         // Conexiones mínimas mantenidas abiertas
	MaxConnLifetime time.Duration // Tiempo máximo de vida de una conexión
	MaxConnIdleTime time.Duration // Tiempo máximo idle antes de cerrar la conexión
}

// NewPool crea y valida un pgxpool.Pool con la configuración dada.
// Ejecuta un Ping() para verificar conectividad antes de retornar.
// Llamado en cmd/{bc}/main.go al arrancar el servicio.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pgutil: parse DSN: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	// Hook ejecutado en cada nueva conexión del pool.
	// Permite registrar tipos custom de Postgres (enums, dominios, etc.)
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return registerTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pgutil: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgutil: ping database: %w", err)
	}

	return pool, nil
}

// registerTypes registra tipos nativos de Postgres para mapeo automático.
// pgx/v5 maneja UUID y JSONB de forma nativa sin configuración adicional.
// Extender aquí para registrar tipos custom (enums de Postgres, etc.).
func registerTypes(_ context.Context, _ *pgx.Conn) error {
	return nil
}
