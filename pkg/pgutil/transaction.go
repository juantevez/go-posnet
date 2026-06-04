package pgutil

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WithTransaction ejecuta fn dentro de una transacción Postgres.
//
// Comportamiento:
//   - Si fn retorna nil  → COMMIT
//   - Si fn retorna error → ROLLBACK automático (vía defer)
//
// El nivel de aislamiento es configurable. Para la mayoría de los
// command handlers usar WithReadCommitted es suficiente.
//
// Uso típico en un handler:
//
//	err := pgutil.WithReadCommitted(ctx, pool, func(tx pgx.Tx) error {
//	    if err := repo.Save(ctx, tx, transaction); err != nil {
//	        return err
//	    }
//	    return idempotency.MarkAsProcessed(ctx, tx, event.EventID)
//	})
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

	// Rollback es no-op si el commit ya ocurrió — seguro de llamar siempre.
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("pgutil: commit transaction: %w", err)
	}
	return nil
}

// WithReadCommitted es un alias conveniente para la mayoría de los handlers.
// Nivel de aislamiento READ COMMITTED: cada query ve los datos commiteados
// hasta ese momento. Suficiente para el 95% de los casos del sistema.
func WithReadCommitted(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return WithTransaction(ctx, pool, pgx.ReadCommitted, fn)
}

// WithRepeatableRead usa nivel REPEATABLE READ: garantiza que las lecturas
// dentro de la transacción ven el mismo snapshot. Útil cuando se necesita
// leer y luego escribir basándose en el valor leído (ej: verificar saldo).
func WithRepeatableRead(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return WithTransaction(ctx, pool, pgx.RepeatableRead, fn)
}

// WithSerializable usa el nivel más estricto: SERIALIZABLE.
// Las transacciones se ejecutan como si fueran completamente secuenciales.
// Usar solo cuando se necesita prevenir anomalías de escritura concurrente
// (ej: dos cierres de lote del mismo terminal al mismo tiempo).
func WithSerializable(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
	return WithTransaction(ctx, pool, pgx.Serializable, fn)
}
