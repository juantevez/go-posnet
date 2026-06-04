// Package pgutil centraliza la conexión, las transacciones y las migraciones
// de PostgreSQL para todos los Bounded Contexts del sistema.
package pgutil

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Conexión ─────────────────────────────────────────────────────────────────

// Config contiene los parámetros del pool de conexiones.
type Config struct {
	DSN             string        // postgresql://user:pass@host:5432/db?sslmode=require
	MaxConns        int32         // Máximo de conexiones (recomendado: 10–25 por servicio)
	MinConns        int32         // Conexiones mínimas mantenidas abiertas
	MaxConnLifetime time.Duration // Tiempo máximo de vida de una conexión
	MaxConnIdleTime time.Duration // Tiempo máximo idle antes de cerrar
}

// NewPool crea y valida un pgxpool.Pool con la configuración dada.
// Ejecuta un Ping() para verificar conectividad antes de retornar.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("pgutil: parse DSN: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	// Configurar tipos nativos de Postgres (UUID, JSONB, etc.)
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return registerTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("pgutil: create pool: %w", err)
	}

	// Verificar conectividad al arrancar.
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pgutil: ping database: %w", err)
	}

	return pool, nil
}

// registerTypes registra tipos nativos de Postgres para mapeo automático.
func registerTypes(_ context.Context, _ *pgx.Conn) error {
	// Aquí se pueden registrar tipos custom de Postgres (enums, etc.)
	// pgx/v5 maneja UUID y JSONB de forma nativa.
	return nil
}

// ─── Unit of Work (Transacciones) ────────────────────────────────────────────

// WithTransaction ejecuta fn dentro de una transacción Postgres.
// Si fn devuelve error → ROLLBACK automático.
// Si fn devuelve nil  → COMMIT.
func WithTransaction(
	ctx context.Context,
	pool *pgxpool.Pool,
	isoLevel pgx.TxIsoLevel,
	fn func(pgx.Tx) error,
) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: isoLevel})
	if err != nil {
		return fmt.Errorf("pgutil: begin transaction: %w", err)
	}

	defer func() {
		// Rollback es no-op si el commit ya ocurrió.
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err // El defer hace rollback.
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgutil: commit transaction: %w", err)
	}
	return nil
}

// WithReadCommitted es un alias conveniente para la mayoría de los handlers.
func WithReadCommitted(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return WithTransaction(ctx, pool, pgx.ReadCommitted, fn)
}

// WithSerializable es para operaciones donde se necesita aislamiento total
// (ej: verificar-y-actualizar un saldo sin condición de carrera).
func WithSerializable(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return WithTransaction(ctx, pool, pgx.Serializable, fn)
}
