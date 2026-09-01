package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/domain/aggregate"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func mustMoney(t *testing.T, cents int64) domain.Money {
	t.Helper()
	m, err := domain.NewMoney(cents, domain.ARS)
	if err != nil {
		t.Fatalf("NewMoney(%d) error = %v", cents, err)
	}
	return m
}

func mustSTAN(t *testing.T, v int) domain.STAN {
	t.Helper()
	s, err := domain.NewSTAN(v)
	if err != nil {
		t.Fatalf("NewSTAN(%d) error = %v", v, err)
	}
	return s
}

func mustPAN(t *testing.T) domain.PAN {
	t.Helper()
	p, err := domain.NewPAN("1234", domain.NetworkVisa)
	if err != nil {
		t.Fatalf("NewPAN() error = %v", err)
	}
	return p
}

func mustAuthCode(t *testing.T, s string) domain.AuthCode {
	t.Helper()
	a, err := domain.NewAuthCode(s)
	if err != nil {
		t.Fatalf("NewAuthCode(%q) error = %v", s, err)
	}
	return a
}

// newValidTransaction crea una Transaction válida en estado RECEIVED, lista
// para ser usada como punto de partida en los tests de transición de estado.
func newValidTransaction(t *testing.T) *aggregate.Transaction {
	t.Helper()
	tx, err := aggregate.NewTransaction(
		domain.NewTransactionID(),
		domain.NewTerminalID(),
		domain.NewMerchantID(),
		mustMoney(t, 5000),
		mustSTAN(t, 1),
		mustPAN(t),
		valueobject.EntryModeChip,
		domain.CardToken{},
		"emv-data-base64",
		[]byte{0xAA, 0xBB},
	)
	if err != nil {
		t.Fatalf("NewTransaction() error = %v", err)
	}
	return tx
}

// newProcessingTransaction avanza una Transaction nueva hasta el estado
// PROCESSING pasando por FRAUD_CHECKING con una decisión de aprobación.
func newProcessingTransaction(t *testing.T) *aggregate.Transaction {
	t.Helper()
	tx := newValidTransaction(t)
	if err := tx.StartFraudCheck(); err != nil {
		t.Fatalf("StartFraudCheck() error = %v", err)
	}
	fd, err := valueobject.NewFraudDecision(10, valueobject.FraudDecisionApprove, nil)
	if err != nil {
		t.Fatalf("NewFraudDecision() error = %v", err)
	}
	if err := tx.ApplyFraudDecision(fd); err != nil {
		t.Fatalf("ApplyFraudDecision() error = %v", err)
	}
	return tx
}

// newApprovedTransaction avanza una Transaction nueva hasta el estado APPROVED.
func newApprovedTransaction(t *testing.T) *aggregate.Transaction {
	t.Helper()
	tx := newProcessingTransaction(t)
	if err := tx.Approve(mustAuthCode(t, "AB1234")); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	return tx
}

// newIndeterminateTransaction avanza una Transaction nueva hasta el estado
// INDETERMINATE.
func newIndeterminateTransaction(t *testing.T) *aggregate.Transaction {
	t.Helper()
	tx := newProcessingTransaction(t)
	if err := tx.MarkIndeterminate(); err != nil {
		t.Fatalf("MarkIndeterminate() error = %v", err)
	}
	return tx
}

// baseReconstituteParams devuelve un ReconstituteParams válido con un
// conjunto fijo de valores, listo para ser ajustado en cada caso de test.
func baseReconstituteParams(t *testing.T) aggregate.ReconstituteParams {
	t.Helper()
	return aggregate.ReconstituteParams{
		ID:            domain.NewTransactionID(),
		TerminalID:    domain.NewTerminalID(),
		MerchantID:    domain.NewMerchantID(),
		Amount:        mustMoney(t, 12345),
		STAN:          mustSTAN(t, 7),
		PAN:           mustPAN(t),
		EntryMode:     valueobject.EntryModeContactless,
		State:         valueobject.StateApproved,
		EMVDataBase64: "emv==",
		ISO8583Raw:    []byte{0x01, 0x02},
		ReceivedAt:    time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
}
