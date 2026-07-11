package natsutil

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/metric"

	"github.com/juantevez/go-posnet/pkg/observability"
)

// idempotencyPool es el subconjunto de *pgxpool.Pool que necesita este
// store — permite testear IsAlreadyProcessed con un pool falso (ej: pgxmock)
// sin depender del tipo concreto de pgx.
type idempotencyPool interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// IdempotencyStore verifica y registra event_ids procesados en Postgres.
// Cada BC tiene su propia tabla processed_events en su schema.
type IdempotencyStore struct {
	pool       idempotencyPool
	schema     string // ej: "authorization", "settlement"
	duplicates metric.Int64Counter
}

// NewIdempotencyStore crea un IdempotencyStore para el schema del BC dado.
//
// Registra el counter posnet_idempotency_duplicates_total (el label "bc" lo
// agrega Prometheus como target label). Debe construirse tras
// observability.InitMeter(); si el meter no está listo, el counter queda nil
// y las mediciones se omiten (nil-safe).
func NewIdempotencyStore(pool idempotencyPool, schema string) *IdempotencyStore {
	dup, _ := observability.Meter("posnet.idempotency").Int64Counter(
		"posnet_idempotency_duplicates",
		metric.WithDescription("Eventos NATS duplicados descartados por idempotencia."),
	)
	return &IdempotencyStore{pool: pool, schema: schema, duplicates: dup}
}

// IsAlreadyProcessed retorna true si el eventID ya fue procesado.
// Consulta la tabla {schema}.processed_events.
func (s *IdempotencyStore) IsAlreadyProcessed(ctx context.Context, eventID string) (bool, error) {
	query := fmt.Sprintf(`SELECT 1 FROM %s.processed_events WHERE event_id = $1`, s.schema)
	var dummy int
	err := s.pool.QueryRow(ctx, query, eventID).Scan(&dummy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // No existe → no procesado
		}
		return false, fmt.Errorf("idempotency: check event_id %q: %w", eventID, err)
	}
	return true, nil // Existe → ya procesado
}

// TryMarkAsProcessed intenta registrar el eventID como procesado dentro de tx.
// Retorna true si el evento fue insertado (es nuevo), false si ya existía (duplicado).
// DEBE llamarse al inicio del callback de WithReadCommitted: si la transacción hace
// rollback, el registro también se revierte, preservando la atomicidad.
func (s *IdempotencyStore) TryMarkAsProcessed(ctx context.Context, tx pgx.Tx, eventID string) (bool, error) {
	query := fmt.Sprintf(
		`INSERT INTO %s.processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING`,
		s.schema,
	)
	res, err := tx.Exec(ctx, query, eventID)
	if err != nil {
		return false, fmt.Errorf("idempotency: mark event_id %q: %w", eventID, err)
	}
	inserted := res.RowsAffected() > 0
	if !inserted && s.duplicates != nil {
		s.duplicates.Add(ctx, 1)
	}
	return inserted, nil
}
