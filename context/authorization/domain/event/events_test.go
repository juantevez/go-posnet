package event_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/domain/event"
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

// withinWindow verifica que ts esté entre before y after, inclusive.
func withinWindow(t *testing.T, ts, before, after time.Time) {
	t.Helper()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp = %v, want between %v and %v", ts, before, after)
	}
}

func TestNewTransactionCreated(t *testing.T) {
	id := domain.NewTransactionID()
	tid := domain.NewTerminalID()
	mid := domain.NewMerchantID()
	amount := mustMoney(t, 5000)
	stan := mustSTAN(t, 1)

	before := time.Now().UTC()
	e := event.NewTransactionCreated(id, tid, mid, amount, stan)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if !e.TerminalID.Equals(tid) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, tid)
	}
	if !e.MerchantID.Equals(mid) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, mid)
	}
	if !e.Amount.Equals(amount) {
		t.Errorf("Amount = %v, want %v", e.Amount, amount)
	}
	if !e.STAN.Equals(stan) {
		t.Errorf("STAN = %v, want %v", e.STAN, stan)
	}
	if e.EventType() != "transaction.created" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.created")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewFraudCheckStarted(t *testing.T) {
	id := domain.NewTransactionID()

	before := time.Now().UTC()
	e := event.NewFraudCheckStarted(id)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if e.EventType() != "fraud.check.started" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "fraud.check.started")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewTransactionApproved(t *testing.T) {
	id := domain.NewTransactionID()
	tid := domain.NewTerminalID()
	mid := domain.NewMerchantID()
	amount := mustMoney(t, 5000)
	pan := mustPAN(t)
	authCode := mustAuthCode(t, "AB1234")
	fraudScore := 12

	before := time.Now().UTC()
	e := event.NewTransactionApproved(id, tid, mid, amount, pan, authCode, fraudScore)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if !e.TerminalID.Equals(tid) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, tid)
	}
	if !e.MerchantID.Equals(mid) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, mid)
	}
	if !e.Amount.Equals(amount) {
		t.Errorf("Amount = %v, want %v", e.Amount, amount)
	}
	if e.PAN != pan {
		t.Errorf("PAN = %v, want %v", e.PAN, pan)
	}
	if !e.AuthCode.Equals(authCode) {
		t.Errorf("AuthCode = %v, want %v", e.AuthCode, authCode)
	}
	if e.FraudScore != fraudScore {
		t.Errorf("FraudScore = %d, want %d", e.FraudScore, fraudScore)
	}
	if e.EventType() != "transaction.approved" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.approved")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewTransactionRejected(t *testing.T) {
	id := domain.NewTransactionID()
	tid := domain.NewTerminalID()
	mid := domain.NewMerchantID()
	rc, err := valueobject.NewRejectionFromISO(valueobject.ISO_DO_NOT_HONOR)
	if err != nil {
		t.Fatalf("NewRejectionFromISO() error = %v", err)
	}

	before := time.Now().UTC()
	e := event.NewTransactionRejected(id, tid, mid, rc)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if !e.TerminalID.Equals(tid) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, tid)
	}
	if !e.MerchantID.Equals(mid) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, mid)
	}
	if e.RejectionCode.Code() != rc.Code() || e.RejectionCode.Source() != rc.Source() {
		t.Errorf("RejectionCode = %v, want %v", e.RejectionCode, rc)
	}
	if e.EventType() != "transaction.rejected" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.rejected")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewTransactionIndeterminate(t *testing.T) {
	id := domain.NewTransactionID()
	tid := domain.NewTerminalID()

	before := time.Now().UTC()
	e := event.NewTransactionIndeterminate(id, tid)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if !e.TerminalID.Equals(tid) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, tid)
	}
	if e.EventType() != "transaction.indeterminate" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.indeterminate")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewTransactionReversed(t *testing.T) {
	id := domain.NewTransactionID()
	tid := domain.NewTerminalID()
	mid := domain.NewMerchantID()
	amount := mustMoney(t, 5000)

	before := time.Now().UTC()
	e := event.NewTransactionReversed(id, tid, mid, amount)
	after := time.Now().UTC()

	if !e.TransactionID.Equals(id) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, id)
	}
	if !e.TerminalID.Equals(tid) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, tid)
	}
	if !e.MerchantID.Equals(mid) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, mid)
	}
	if !e.Amount.Equals(amount) {
		t.Errorf("Amount = %v, want %v", e.Amount, amount)
	}
	if e.EventType() != "transaction.reversed" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.reversed")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

// TestEvents_ImplementDomainEventInterface verifica que todos los eventos
// satisfacen la interfaz DomainEvent en tiempo de compilación.
func TestEvents_ImplementDomainEventInterface(t *testing.T) {
	var events []event.DomainEvent
	events = append(events,
		event.NewTransactionCreated(domain.NewTransactionID(), domain.NewTerminalID(), domain.NewMerchantID(), mustMoney(t, 100), mustSTAN(t, 1)),
		event.NewFraudCheckStarted(domain.NewTransactionID()),
		event.NewTransactionApproved(domain.NewTransactionID(), domain.NewTerminalID(), domain.NewMerchantID(), mustMoney(t, 100), mustPAN(t), mustAuthCode(t, "AB1234"), 10),
		event.NewTransactionIndeterminate(domain.NewTransactionID(), domain.NewTerminalID()),
		event.NewTransactionReversed(domain.NewTransactionID(), domain.NewTerminalID(), domain.NewMerchantID(), mustMoney(t, 100)),
	)

	for _, e := range events {
		if e.EventType() == "" {
			t.Errorf("%T.EventType() = \"\", want non-empty", e)
		}
		if e.OccurredAt().IsZero() {
			t.Errorf("%T.OccurredAt() is zero, want set", e)
		}
	}
}
