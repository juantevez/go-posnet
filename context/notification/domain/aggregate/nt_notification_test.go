package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/event"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── helpers ────────────────────────────────────────────────────────────────

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
	n, err := aggregate.NewNotification(
		domain.NewTransactionID(),
		domain.NewMerchantID(),
		valueobject.ChannelWebhook,
		mustReceipt(t),
	)
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}
	return n
}

// newPendingNotificationWithMaxAttempts arma una Notification en PENDING con
// un MaxAttempts custom, vía Reconstitute — útil para no tener que llamar
// MarkFailed 5 veces (el default) en los tests de backoff/DEAD.
func newPendingNotificationWithMaxAttempts(t *testing.T, maxAttempts int) *aggregate.Notification {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            domain.NewTransactionID().String(),
		TransactionID: domain.NewTransactionID(),
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelWebhook,
		State:         valueobject.StatePending,
		Receipt:       mustReceipt(t),
		MaxAttempts:   maxAttempts,
		CreatedAt:     time.Now().UTC(),
	})
}

// ─── NewNotification ──────────────────────────────────────────────────────────

func TestNewNotification_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	merchantID := domain.NewMerchantID()
	receipt := mustReceipt(t)

	before := time.Now().UTC()
	n, err := aggregate.NewNotification(txID, merchantID, valueobject.ChannelWebhook, receipt)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}

	if n.ID() == "" {
		t.Error("ID() is empty, want a generated UUID")
	}
	if !n.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", n.TransactionID(), txID)
	}
	if !n.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", n.MerchantID(), merchantID)
	}
	if n.Channel() != valueobject.ChannelWebhook {
		t.Errorf("Channel() = %v, want %v", n.Channel(), valueobject.ChannelWebhook)
	}
	if n.State() != valueobject.StatePending {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StatePending)
	}
	if n.Receipt() != receipt {
		t.Errorf("Receipt() = %v, want %v", n.Receipt(), receipt)
	}
	if n.MaxAttempts() != 5 {
		t.Errorf("MaxAttempts() = %d, want 5", n.MaxAttempts())
	}
	if n.AttemptCount() != 0 {
		t.Errorf("AttemptCount() = %d, want 0", n.AttemptCount())
	}
	if n.CreatedAt().Before(before) || n.CreatedAt().After(after) {
		t.Errorf("CreatedAt() = %v, want between %v and %v", n.CreatedAt(), before, after)
	}
	if n.DispatchedAt() != nil {
		t.Errorf("DispatchedAt() = %v, want nil", n.DispatchedAt())
	}
	if n.NextRetryAt() != nil {
		t.Errorf("NextRetryAt() = %v, want nil", n.NextRetryAt())
	}
	if len(n.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() = %v, want empty", n.DomainEvents())
	}
}

func TestNewNotification_ValidationError(t *testing.T) {
	_, err := aggregate.NewNotification(domain.TransactionID{}, domain.NewMerchantID(), valueobject.ChannelWebhook, mustReceipt(t))
	if err == nil {
		t.Fatal("NewNotification() error = nil, want error")
	}
}

// ─── MarkSent ───────────────────────────────────────────────────────────────

func TestMarkSent_Success(t *testing.T) {
	n := newPendingNotification(t)

	before := time.Now().UTC()
	if err := n.MarkSent(200); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	after := time.Now().UTC()

	if n.State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StateSent)
	}
	if n.AttemptCount() != 1 {
		t.Fatalf("AttemptCount() = %d, want 1", n.AttemptCount())
	}
	att := n.Attempts()[0]
	if !att.Success() {
		t.Error("Attempts()[0].Success() = false, want true")
	}
	if att.HTTPStatus() != 200 {
		t.Errorf("Attempts()[0].HTTPStatus() = %d, want 200", att.HTTPStatus())
	}
	if att.AttemptNumber() != 1 {
		t.Errorf("Attempts()[0].AttemptNumber() = %d, want 1", att.AttemptNumber())
	}

	dispatchedAt := n.DispatchedAt()
	if dispatchedAt == nil {
		t.Fatal("DispatchedAt() = nil, want non-nil")
	}
	if dispatchedAt.Before(before) || dispatchedAt.After(after) {
		t.Errorf("DispatchedAt() = %v, want between %v and %v", dispatchedAt, before, after)
	}

	events := n.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() = %d, want 1", len(events))
	}
	dispatched, ok := events[0].(event.NotificationDispatched)
	if !ok {
		t.Fatalf("DomainEvents()[0] type = %T, want event.NotificationDispatched", events[0])
	}
	if dispatched.NotificationID != n.ID() {
		t.Errorf("NotificationDispatched.NotificationID = %q, want %q", dispatched.NotificationID, n.ID())
	}
	if dispatched.Attempts != 1 {
		t.Errorf("NotificationDispatched.Attempts = %d, want 1", dispatched.Attempts)
	}
}

func TestMarkSent_FromRetrying(t *testing.T) {
	n := newPendingNotificationWithMaxAttempts(t, 5)
	if err := n.MarkFailed(500, "timeout"); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}
	if n.State() != valueobject.StateRetrying {
		t.Fatalf("State() = %v, want %v", n.State(), valueobject.StateRetrying)
	}

	if err := n.MarkSent(200); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if n.State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StateSent)
	}
	if n.AttemptCount() != 2 {
		t.Errorf("AttemptCount() = %d, want 2 (1 fallido + 1 exitoso)", n.AttemptCount())
	}
}

func TestMarkSent_InvalidTransition(t *testing.T) {
	n := newPendingNotification(t)
	if err := n.MarkSent(200); err != nil {
		t.Fatalf("first MarkSent() error = %v", err)
	}
	if err := n.MarkSent(200); err == nil {
		t.Fatal("second MarkSent() error = nil, want error (SENT es terminal)")
	}
}

// ─── MarkFailed ───────────────────────────────────────────────────────────────

func TestMarkFailed_FirstFailureGoesToRetrying(t *testing.T) {
	n := newPendingNotificationWithMaxAttempts(t, 5)

	before := time.Now().UTC()
	if err := n.MarkFailed(500, "connection timeout"); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	if n.State() != valueobject.StateRetrying {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StateRetrying)
	}
	if n.AttemptCount() != 1 {
		t.Fatalf("AttemptCount() = %d, want 1", n.AttemptCount())
	}
	att := n.Attempts()[0]
	if att.Success() {
		t.Error("Attempts()[0].Success() = true, want false")
	}
	if att.ErrorMessage() != "connection timeout" {
		t.Errorf("Attempts()[0].ErrorMessage() = %q, want %q", att.ErrorMessage(), "connection timeout")
	}

	nextRetryAt := n.NextRetryAt()
	if nextRetryAt == nil {
		t.Fatal("NextRetryAt() = nil, want non-nil")
	}
	wantBackoff := 30 * time.Second
	got := nextRetryAt.Sub(before)
	if got < wantBackoff || got > wantBackoff+time.Second {
		t.Errorf("backoff = %v, want ~%v", got, wantBackoff)
	}

	// El primer fallo no debe emitir ningún evento de dominio — solo DEAD
	// dispara un evento, y todavía no llegamos ahí.
	if len(n.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() = %v, want empty", n.DomainEvents())
	}
}

func TestMarkFailed_BackoffProgression(t *testing.T) {
	n := newPendingNotificationWithMaxAttempts(t, 10) // alto para no llegar a DEAD
	wantBackoffs := []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute, time.Hour, time.Hour}

	for i, want := range wantBackoffs {
		before := time.Now().UTC()
		if err := n.MarkFailed(500, "timeout"); err != nil {
			t.Fatalf("MarkFailed() attempt %d error = %v", i+1, err)
		}
		if n.State() != valueobject.StateRetrying {
			t.Fatalf("attempt %d: State() = %v, want RETRYING", i+1, n.State())
		}
		got := n.NextRetryAt().Sub(before)
		if got < want || got > want+time.Second {
			t.Errorf("attempt %d: backoff = %v, want ~%v", i+1, got, want)
		}
	}
}

func TestMarkFailed_ReachesDeadAfterMaxAttempts(t *testing.T) {
	n := newPendingNotificationWithMaxAttempts(t, 2)

	if err := n.MarkFailed(500, "timeout 1"); err != nil {
		t.Fatalf("MarkFailed() attempt 1 error = %v", err)
	}
	if n.State() != valueobject.StateRetrying {
		t.Fatalf("State() after attempt 1 = %v, want RETRYING", n.State())
	}

	if err := n.MarkFailed(500, "timeout 2"); err != nil {
		t.Fatalf("MarkFailed() attempt 2 error = %v", err)
	}
	if n.State() != valueobject.StateDead {
		t.Fatalf("State() after attempt 2 = %v, want DEAD", n.State())
	}
	if n.AttemptCount() != 2 {
		t.Errorf("AttemptCount() = %d, want 2", n.AttemptCount())
	}

	events := n.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() = %d, want 1", len(events))
	}
	dead, ok := events[0].(event.NotificationDead)
	if !ok {
		t.Fatalf("DomainEvents()[0] type = %T, want event.NotificationDead", events[0])
	}
	if dead.TotalAttempts != 2 {
		t.Errorf("NotificationDead.TotalAttempts = %d, want 2", dead.TotalAttempts)
	}
	if dead.NotificationID != n.ID() {
		t.Errorf("NotificationDead.NotificationID = %q, want %q", dead.NotificationID, n.ID())
	}
}

func TestMarkFailed_InvalidFromTerminalState(t *testing.T) {
	n := newPendingNotification(t)
	if err := n.MarkSent(200); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if err := n.MarkFailed(500, "too late"); err == nil {
		t.Fatal("MarkFailed() error = nil, want error (SENT es terminal)")
	}
}

// ─── DomainEvents / ClearDomainEvents ─────────────────────────────────────────

func TestNotification_ClearDomainEvents(t *testing.T) {
	n := newPendingNotification(t)
	if err := n.MarkSent(200); err != nil {
		t.Fatalf("MarkSent() error = %v", err)
	}
	if len(n.DomainEvents()) == 0 {
		t.Fatal("DomainEvents() = empty, want at least the dispatched event")
	}
	n.ClearDomainEvents()
	if len(n.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() after ClearDomainEvents() = %v, want empty", n.DomainEvents())
	}
}
