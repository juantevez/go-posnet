package postgres_test

import (
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/entity"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
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

// ─── builders de dominio ──────────────────────────────────────────────────────

func mustMoney(t *testing.T) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(1000, domain.ARS)
	if err != nil {
		t.Fatalf("NewMoney() error = %v", err)
	}
	return m
}

func mustSTAN(t *testing.T) domain.STAN {
	t.Helper()
	s, err := domain.NewSTAN(123456)
	if err != nil {
		t.Fatalf("NewSTAN() error = %v", err)
	}
	return s
}

func newAwaitingSession(t *testing.T) *aggregate.PaymentSession {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelNFC,
		State:      valueobject.StateAwaitingPayment,
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
		CreatedAt:  time.Now().UTC(),
	})
}

func newApprovedSession(t *testing.T) *aggregate.PaymentSession {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelNFC,
		State:      valueobject.StateApproved,
		AuthCode:   "AUTH123",
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
		CreatedAt:  time.Now().UTC(),
	})
}

func newTerminal(t *testing.T) *entity.Terminal {
	t.Helper()
	return entity.ReconstitueTerminal(
		domain.NewTerminalID(), domain.NewMerchantID(),
		"TRM-0042", "terminal-0042.posnet.local",
		entity.TerminalActive,
		time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	)
}

// ─── fixtures de filas de Postgres ────────────────────────────────────────────

var sessionColumns = []string{
	"id", "terminal_id", "merchant_id",
	"state", "channel",
	"amount_cents", "currency", "stan",
	"auth_code", "rejection_code", "rejection_reason",
	"expires_at", "created_at", "closed_at",
}

// sessionRow arma una fila de terminal_gateway.payment_sessions. Los tipos
// deben calzar exactamente con los destinos de Scan en scanSession: pgxmock
// no convierte tipos como pgx real.
type sessionRow struct {
	id, terminalID, merchantID   string
	state, channel               string
	amountCents                  int64
	currency                     string
	stan                         int
	authCode, rejCode, rejReason *string
	expiresAt, createdAt         time.Time
	closedAt                     *time.Time
}

func newSessionRow() sessionRow {
	return sessionRow{
		id:          domain.NewTransactionID().String(),
		terminalID:  domain.NewTerminalID().String(),
		merchantID:  domain.NewMerchantID().String(),
		state:       string(valueobject.StateAwaitingPayment),
		channel:     string(valueobject.ChannelNFC),
		amountCents: 1000,
		currency:    "ARS",
		stan:        123456,
		expiresAt:   time.Date(2026, 1, 15, 10, 5, 0, 0, time.UTC),
		createdAt:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}
}

func (r sessionRow) rows() *pgxmock.Rows {
	return pgxmock.NewRows(sessionColumns).AddRow(
		r.id, r.terminalID, r.merchantID,
		r.state, r.channel,
		r.amountCents, r.currency, r.stan,
		r.authCode, r.rejCode, r.rejReason,
		r.expiresAt, r.createdAt, r.closedAt,
	)
}

var terminalColumns = []string{
	"id", "merchant_id", "terminal_code", "certificate_cn", "status", "created_at", "updated_at",
}

type terminalRow struct {
	id, merchantID, code, cn, status string
	createdAt, updatedAt             time.Time
}

func newTerminalRow() terminalRow {
	return terminalRow{
		id:         domain.NewTerminalID().String(),
		merchantID: domain.NewMerchantID().String(),
		code:       "TRM-0042",
		cn:         "terminal-0042.posnet.local",
		status:     string(entity.TerminalActive),
		createdAt:  time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
		updatedAt:  time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}
}

func (r terminalRow) rows() *pgxmock.Rows {
	return pgxmock.NewRows(terminalColumns).AddRow(
		r.id, r.merchantID, r.code, r.cn, r.status, r.createdAt, r.updatedAt,
	)
}
