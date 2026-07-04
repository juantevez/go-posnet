// Package outbox implementa el patrón Transactional Outbox para garantizar
// que los eventos se publiquen a NATS exactamente una vez, incluso ante fallos
// del pod entre el Save de Postgres y el Publish a JetStream.
//
// Flujo:
//  1. El handler escribe el evento en la tabla outbox dentro de la misma TX
//     que persiste el aggregate (Store.InsertTx).
//  2. El Relay corre en background, lee filas pendientes con FOR UPDATE SKIP LOCKED
//     y las publica a JetStream. Si el pod muere entre ambos pasos, el Relay
//     las reintenta en el próximo ciclo; JetStream deduplica via Nats-Msg-Id.
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	nats "github.com/nats-io/nats.go"
)

// pgxPool es el subconjunto de *pgxpool.Pool que el Relay necesita — permite
// testearlo con un pool falso (ej: pgxmock) sin depender del tipo concreto.
type pgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Store escribe entradas en la tabla outbox dentro de una transacción existente.
type Store struct {
	schema string
}

// NewStore crea un Store para el schema del BC dado (ej: "terminal_gateway").
func NewStore(schema string) *Store {
	return &Store{schema: schema}
}

// InsertTx inserta un mensaje pendiente en el outbox dentro de la transacción tx.
// Debe llamarse en la misma TX que persiste el aggregate para garantizar atomicidad.
func (s *Store) InsertTx(ctx context.Context, tx pgx.Tx, subject, eventID string, payload []byte) error {
	q := fmt.Sprintf(
		`INSERT INTO %s.outbox (subject, event_id, payload) VALUES ($1, $2, $3)`,
		s.schema,
	)
	if _, err := tx.Exec(ctx, q, subject, eventID, payload); err != nil {
		return fmt.Errorf("outbox: insert %q: %w", eventID, err)
	}
	return nil
}

// ─── Relay ────────────────────────────────────────────────────────────────────

// Relay lee entradas pendientes del outbox y las publica a JetStream.
// Múltiples instancias pueden correr en paralelo de forma segura: FOR UPDATE SKIP LOCKED
// garantiza que dos Relays no procesen la misma fila.
type Relay struct {
	pool     pgxPool
	js       nats.JetStreamContext
	schema   string
	interval time.Duration
	batch    int
}

// NewRelay crea un Relay que sondea cada interval y procesa hasta batch filas por ciclo.
func NewRelay(pool pgxPool, js nats.JetStreamContext, schema string, interval time.Duration, batch int) *Relay {
	return &Relay{pool: pool, js: js, schema: schema, interval: interval, batch: batch}
}

// Run corre el loop de publicación hasta que ctx sea cancelado.
// Llamar en una goroutine independiente.
func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.publishPending(ctx); err != nil {
				slog.ErrorContext(ctx, "outbox relay: publish cycle failed",
					slog.String("schema", r.schema),
					slog.String("error", err.Error()),
				)
			}
		}
	}
}

type outboxRow struct {
	id      string
	subject string
	eventID string
	payload []byte
}

// publishPending lee el próximo batch y publica cada mensaje a JetStream.
// Toda la operación corre dentro de una sola TX:
//   - SELECT FOR UPDATE SKIP LOCKED garantiza exclusión mutua entre instancias.
//   - Si el publish falla, la TX hace rollback y las filas se reintentan en el próximo ciclo.
//   - Si el publish ya llegó a JetStream pero el commit falla, el reintento es seguro
//     porque JetStream deduplica por Nats-Msg-Id (= event_id).
func (r *Relay) publishPending(ctx context.Context) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("outbox relay: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := fmt.Sprintf(
		`SELECT id, subject, event_id, payload
		 FROM %s.outbox
		 ORDER BY created_at
		 LIMIT %d
		 FOR UPDATE SKIP LOCKED`,
		r.schema, r.batch,
	)
	rows, err := tx.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("outbox relay: query pending: %w", err)
	}

	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.id, &row.subject, &row.eventID, &row.payload); err != nil {
			rows.Close()
			return fmt.Errorf("outbox relay: scan row: %w", err)
		}
		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("outbox relay: rows error: %w", err)
	}

	if len(pending) == 0 {
		return tx.Commit(ctx)
	}

	for _, row := range pending {
		msg := &nats.Msg{
			Subject: row.subject,
			Data:    row.payload,
			Header:  make(nats.Header),
		}
		msg.Header.Set(nats.MsgIdHdr, row.eventID)

		if _, err := r.js.PublishMsg(msg); err != nil {
			return fmt.Errorf("outbox relay: publish %q: %w", row.eventID, err)
		}

		delQ := fmt.Sprintf(`DELETE FROM %s.outbox WHERE id = $1`, r.schema)
		if _, err := tx.Exec(ctx, delQ, row.id); err != nil {
			return fmt.Errorf("outbox relay: delete %s: %w", row.id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("outbox relay: commit: %w", err)
	}

	slog.Info("outbox relay: published batch",
		slog.String("schema", r.schema),
		slog.Int("count", len(pending)),
	)
	return nil
}
