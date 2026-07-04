package aggregate_test

import (
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/event"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── builders ────────────────────────────────────────────────────────────────

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

func newSession(t *testing.T) *aggregate.PaymentSession {
	t.Helper()
	s, err := aggregate.NewPaymentSession(
		domain.NewTerminalID(), domain.NewMerchantID(), mustMoney(t), mustSTAN(t), valueobject.ChannelQR,
	)
	if err != nil {
		t.Fatalf("NewPaymentSession() error = %v", err)
	}
	s.ClearDomainEvents()
	return s
}

// processingSession construye una sesión ya en estado PROCESSING, con
// ExpiresAt en el futuro para no interferir con los tests de transición.
func processingSession(t *testing.T) *aggregate.PaymentSession {
	t.Helper()
	s := newSession(t)
	if err := s.StartProcessing([]byte("iso8583-raw"), "emv-base64"); err != nil {
		t.Fatalf("StartProcessing() error = %v", err)
	}
	s.ClearDomainEvents()
	return s
}

// ─── NewPaymentSession ─────────────────────────────────────────────────────────

func TestNewPaymentSession_Success(t *testing.T) {
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	amount := mustMoney(t)
	stan := mustSTAN(t)

	before := time.Now().UTC()
	s, err := aggregate.NewPaymentSession(terminalID, merchantID, amount, stan, valueobject.ChannelQR)
	if err != nil {
		t.Fatalf("NewPaymentSession() error = %v", err)
	}
	after := time.Now().UTC()

	if s.ID().IsZero() {
		t.Error("ID() is zero, want a generated TransactionID")
	}
	if !s.TerminalID().Equals(terminalID) {
		t.Errorf("TerminalID() = %v, want %v", s.TerminalID(), terminalID)
	}
	if !s.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", s.MerchantID(), merchantID)
	}
	if !s.Amount().Equals(amount) {
		t.Errorf("Amount() = %v, want %v", s.Amount(), amount)
	}
	if s.STAN() != stan {
		t.Errorf("STAN() = %v, want %v", s.STAN(), stan)
	}
	if s.Channel() != valueobject.ChannelQR {
		t.Errorf("Channel() = %v, want %v", s.Channel(), valueobject.ChannelQR)
	}
	if s.State() != valueobject.StateAwaitingPayment {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateAwaitingPayment)
	}
	if s.AuthCode() != "" || s.RejectionCode() != "" || s.RejectionReason() != "" {
		t.Error("AuthCode/RejectionCode/RejectionReason should be empty right after creation")
	}
	if s.ClosedAt() != nil {
		t.Error("ClosedAt() should be nil right after creation")
	}
	if s.CreatedAt().Before(before) || s.CreatedAt().After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", s.CreatedAt(), before, after)
	}
	wantExpiresAt := s.CreatedAt().Add(5 * time.Minute)
	if !s.ExpiresAt().Equal(wantExpiresAt) {
		t.Errorf("ExpiresAt() = %v, want %v (CreatedAt + 5m)", s.ExpiresAt(), wantExpiresAt)
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	created, ok := events[0].(event.SessionCreated)
	if !ok {
		t.Fatalf("event type = %T, want event.SessionCreated", events[0])
	}
	if created.TransactionID != s.ID() {
		t.Errorf("SessionCreated.TransactionID = %v, want %v", created.TransactionID, s.ID())
	}
	if !created.Amount.Equals(amount) {
		t.Errorf("SessionCreated.Amount = %v, want %v", created.Amount, amount)
	}
}

func TestNewPaymentSession_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		terminalID domain.TerminalID
		merchantID domain.MerchantID
		amount     domain.Money
		wantSubstr string
	}{
		{"zero terminal_id", domain.TerminalID{}, domain.NewMerchantID(), mustMoney(t), "terminal_id"},
		{"zero merchant_id", domain.NewTerminalID(), domain.MerchantID{}, mustMoney(t), "merchant_id"},
		{"non-positive amount", domain.NewTerminalID(), domain.NewMerchantID(), domain.Money{}, "amount must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := aggregate.NewPaymentSession(tc.terminalID, tc.merchantID, tc.amount, mustSTAN(t), valueobject.ChannelQR)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantSubstr)
			}
		})
	}
}

// ─── StartProcessing ───────────────────────────────────────────────────────────

func TestStartProcessing_Success(t *testing.T) {
	s := newSession(t)

	if err := s.StartProcessing([]byte("iso8583-raw"), "emv-base64"); err != nil {
		t.Fatalf("StartProcessing() error = %v", err)
	}
	if s.State() != valueobject.StateProcessing {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateProcessing)
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	initiated, ok := events[0].(event.TransactionInitiated)
	if !ok {
		t.Fatalf("event type = %T, want event.TransactionInitiated", events[0])
	}
	if string(initiated.ISO8583Raw) != "iso8583-raw" {
		t.Errorf("ISO8583Raw = %q, want %q", initiated.ISO8583Raw, "iso8583-raw")
	}
	if initiated.EMVDataBase64 != "emv-base64" {
		t.Errorf("EMVDataBase64 = %q, want %q", initiated.EMVDataBase64, "emv-base64")
	}
	if initiated.TransactionID != s.ID() {
		t.Errorf("TransactionID = %v, want %v", initiated.TransactionID, s.ID())
	}
}

func TestStartProcessing_ExpiredSession(t *testing.T) {
	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelQR,
		State:      valueobject.StateAwaitingPayment,
		ExpiresAt:  time.Now().UTC().Add(-time.Minute), // ya venció
		CreatedAt:  time.Now().UTC().Add(-10 * time.Minute),
	})

	err := s.StartProcessing(nil, "")
	if err == nil || !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("error = %v, want it to mention session expired", err)
	}
	if len(s.DomainEvents()) != 0 {
		t.Error("no domain event should be recorded when the session already expired")
	}
}

func TestStartProcessing_InvalidTransition(t *testing.T) {
	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelQR,
		State:      valueobject.StateApproved, // terminal — no puede ir a PROCESSING
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
		CreatedAt:  time.Now().UTC(),
	})

	err := s.StartProcessing(nil, "")
	if err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("error = %v, want it to mention the invalid transition", err)
	}
}

// ─── Approve ──────────────────────────────────────────────────────────────────

func TestApprove_Success(t *testing.T) {
	s := processingSession(t)

	if err := s.Approve("AUTH123"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if s.State() != valueobject.StateApproved {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateApproved)
	}
	if s.AuthCode() != "AUTH123" {
		t.Errorf("AuthCode() = %q, want %q", s.AuthCode(), "AUTH123")
	}
	if s.ClosedAt() == nil {
		t.Fatal("ClosedAt() is nil, want it set")
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	approved, ok := events[0].(event.SessionApproved)
	if !ok {
		t.Fatalf("event type = %T, want event.SessionApproved", events[0])
	}
	if approved.AuthCode != "AUTH123" {
		t.Errorf("SessionApproved.AuthCode = %q, want %q", approved.AuthCode, "AUTH123")
	}
}

func TestApprove_InvalidTransition(t *testing.T) {
	s := newSession(t) // AWAITING_PAYMENT — no puede ir directo a APPROVED
	err := s.Approve("AUTH123")
	if err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("error = %v, want it to mention the invalid transition", err)
	}
}

// ─── Reject ───────────────────────────────────────────────────────────────────

func TestReject_Success(t *testing.T) {
	s := processingSession(t)

	if err := s.Reject("05", "Do not honor"); err != nil {
		t.Fatalf("Reject() error = %v", err)
	}
	if s.State() != valueobject.StateRejected {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateRejected)
	}
	if s.RejectionCode() != "05" || s.RejectionReason() != "Do not honor" {
		t.Errorf("RejectionCode/Reason = %q/%q, want %q/%q", s.RejectionCode(), s.RejectionReason(), "05", "Do not honor")
	}
	if s.ClosedAt() == nil {
		t.Fatal("ClosedAt() is nil, want it set")
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	if _, ok := events[0].(event.SessionRejected); !ok {
		t.Fatalf("event type = %T, want event.SessionRejected", events[0])
	}
}

func TestReject_InvalidTransition(t *testing.T) {
	s := newSession(t)
	err := s.Reject("05", "Do not honor")
	if err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("error = %v, want it to mention the invalid transition", err)
	}
}

// ─── Expire ───────────────────────────────────────────────────────────────────

func TestExpire_Success(t *testing.T) {
	s := newSession(t)

	if err := s.Expire(); err != nil {
		t.Fatalf("Expire() error = %v", err)
	}
	if s.State() != valueobject.StateExpired {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateExpired)
	}
	if s.ClosedAt() == nil {
		t.Fatal("ClosedAt() is nil, want it set")
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	if _, ok := events[0].(event.SessionExpired); !ok {
		t.Fatalf("event type = %T, want event.SessionExpired", events[0])
	}
}

func TestExpire_InvalidTransition(t *testing.T) {
	s := processingSession(t) // PROCESSING no puede ir directo a EXPIRED
	err := s.Expire()
	if err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("error = %v, want it to mention the invalid transition", err)
	}
}

// ─── Cancel ───────────────────────────────────────────────────────────────────

func TestCancel_Success(t *testing.T) {
	s := newSession(t)

	if err := s.Cancel(); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if s.State() != valueobject.StateCancelled {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateCancelled)
	}
	if s.ClosedAt() == nil {
		t.Fatal("ClosedAt() is nil, want it set")
	}

	events := s.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() len = %d, want 1", len(events))
	}
	if _, ok := events[0].(event.SessionCancelled); !ok {
		t.Fatalf("event type = %T, want event.SessionCancelled", events[0])
	}
}

func TestCancel_InvalidTransition(t *testing.T) {
	s := processingSession(t) // PROCESSING no puede ir directo a CANCELLED
	err := s.Cancel()
	if err == nil || !strings.Contains(err.Error(), "invalid transition") {
		t.Fatalf("error = %v, want it to mention the invalid transition", err)
	}
}

// ─── IsExpired / TTLRemaining ──────────────────────────────────────────────────

func TestIsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"in the past", time.Now().UTC().Add(-time.Minute), true},
		{"in the future", time.Now().UTC().Add(time.Minute), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := aggregate.Reconstitute(aggregate.ReconstituteParams{
				ID: domain.NewTransactionID(), TerminalID: domain.NewTerminalID(), MerchantID: domain.NewMerchantID(),
				Amount: mustMoney(t), STAN: mustSTAN(t), Channel: valueobject.ChannelQR,
				State: valueobject.StateAwaitingPayment, ExpiresAt: tc.expiresAt, CreatedAt: time.Now().UTC(),
			})
			if got := s.IsExpired(); got != tc.want {
				t.Errorf("IsExpired() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTTLRemaining_Future(t *testing.T) {
	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID: domain.NewTransactionID(), TerminalID: domain.NewTerminalID(), MerchantID: domain.NewMerchantID(),
		Amount: mustMoney(t), STAN: mustSTAN(t), Channel: valueobject.ChannelQR,
		State: valueobject.StateAwaitingPayment, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), CreatedAt: time.Now().UTC(),
	})

	remaining := s.TTLRemaining()
	if remaining <= 0 || remaining > 2*time.Minute {
		t.Errorf("TTLRemaining() = %v, want a positive value close to 2m", remaining)
	}
}

func TestTTLRemaining_ClampsToZeroWhenExpired(t *testing.T) {
	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID: domain.NewTransactionID(), TerminalID: domain.NewTerminalID(), MerchantID: domain.NewMerchantID(),
		Amount: mustMoney(t), STAN: mustSTAN(t), Channel: valueobject.ChannelQR,
		State: valueobject.StateAwaitingPayment, ExpiresAt: time.Now().UTC().Add(-time.Hour), CreatedAt: time.Now().UTC(),
	})

	if got := s.TTLRemaining(); got != 0 {
		t.Errorf("TTLRemaining() = %v, want 0 (clamped)", got)
	}
}

// ─── ClearDomainEvents ──────────────────────────────────────────────────────────

func TestClearDomainEvents(t *testing.T) {
	s := newSession(t)
	if err := s.Cancel(); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if len(s.DomainEvents()) == 0 {
		t.Fatal("DomainEvents() is empty before ClearDomainEvents(), want at least 1")
	}

	s.ClearDomainEvents()
	if len(s.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() len = %d after ClearDomainEvents(), want 0", len(s.DomainEvents()))
	}
}
