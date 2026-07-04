package postgres_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// newMockPool crea un pool pgxmock y registra su cierre y la verificación de
// expectations al finalizar el test.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations: %v", err)
		}
	})
	return pool
}

// anyArgs devuelve n comodines pgxmock.AnyArg().
func anyArgs(n int) []any {
	args := make([]any, n)
	for i := range args {
		args[i] = pgxmock.AnyArg()
	}
	return args
}

// jsonArg matchea un argumento []byte comparando su contenido JSON decodificado
// contra want, en vez de exigir igualdad byte a byte.
type jsonArg struct{ want any }

func (j jsonArg) Match(v any) bool {
	raw, ok := v.([]byte)
	if !ok {
		return false
	}
	var got any
	if err := json.Unmarshal(raw, &got); err != nil {
		return false
	}
	wantRaw, err := json.Marshal(j.want)
	if err != nil {
		return false
	}
	var want any
	if err := json.Unmarshal(wantRaw, &want); err != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

// ─── builders ────────────────────────────────────────────────────────────────

func mustReceipt(t *testing.T) valueobject.ReceiptPayload {
	t.Helper()
	r, err := valueobject.NewReceiptPayload(
		domain.NewTransactionID().String(), "Merchant Inc", "TERM-001",
		"APPROVED", 5000, "ARS", "1234", "VISA", "CHIP",
		"2026-01-01T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("NewReceiptPayload() error = %v", err)
	}
	return r
}

func newPendingNotification(t *testing.T) *aggregate.Notification {
	t.Helper()
	n, err := aggregate.NewNotification(domain.NewTransactionID(), domain.NewMerchantID(), valueobject.ChannelWebhook, mustReceipt(t))
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}
	return n
}

func newSentNotification(t *testing.T) *aggregate.Notification {
	t.Helper()
	n := newPendingNotification(t)
	if err := n.MarkSent(200); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	return n
}

// ─── fixture de filas de Postgres ────────────────────────────────────────────

var notificationColumns = []string{
	"id", "transaction_id", "merchant_id", "channel", "state", "receipt",
	"attempt_count", "max_attempts", "next_retry_at", "created_at", "dispatched_at",
}

var attemptColumns = []string{
	"id", "notification_id", "attempt_number", "success", "http_status", "error_message", "attempted_at",
}

// notificationRow arma una fila de notification.notifications. Los tipos deben
// calzar exactamente con los destinos de Scan en rawNotification: pgxmock no
// convierte tipos como pgx real.
type notificationRow struct {
	id, transactionID, merchantID string
	channel, state                string
	receiptJSON                   []byte
	attemptCount, maxAttempts     int
	nextRetryAt                   *time.Time
	createdAt                     time.Time
	dispatchedAt                  *time.Time
}

func newNotificationRow(t *testing.T) notificationRow {
	t.Helper()
	receiptJSON, err := json.Marshal(mustReceipt(t))
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return notificationRow{
		id:            "notif-1",
		transactionID: domain.NewTransactionID().String(),
		merchantID:    domain.NewMerchantID().String(),
		channel:       "WEBHOOK",
		state:         "PENDING",
		receiptJSON:   receiptJSON,
		attemptCount:  0,
		maxAttempts:   5,
		createdAt:     time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
}

func (r notificationRow) rows() *pgxmock.Rows {
	return pgxmock.NewRows(notificationColumns).AddRow(
		r.id, r.transactionID, r.merchantID, r.channel, r.state, r.receiptJSON,
		r.attemptCount, r.maxAttempts, r.nextRetryAt, r.createdAt, r.dispatchedAt,
	)
}

func emptyAttemptRows() *pgxmock.Rows {
	return pgxmock.NewRows(attemptColumns)
}

func oneAttemptRow(notifID string) *pgxmock.Rows {
	return pgxmock.NewRows(attemptColumns).AddRow(
		notifID+"-1", notifID, 1, true, 200, "", time.Date(2026, 1, 1, 10, 1, 0, 0, time.UTC),
	)
}
