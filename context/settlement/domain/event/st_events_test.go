package event_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/event"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// withinWindow verifica que ts esté entre before y after, inclusive.
func withinWindow(t *testing.T, ts, before, after time.Time) {
	t.Helper()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp = %v, want between %v and %v", ts, before, after)
	}
}

func mustSummary(t *testing.T) valueobject.BatchSummary {
	t.Helper()
	total, err := domain.NewMoney(2999, domain.ARS)
	if err != nil {
		t.Fatalf("NewMoney() error = %v", err)
	}
	purchase, err := domain.NewMoney(3000, domain.ARS)
	if err != nil {
		t.Fatalf("NewMoney() error = %v", err)
	}
	reversal, err := domain.NewMoney(1, domain.ARS)
	if err != nil {
		t.Fatalf("NewMoney() error = %v", err)
	}
	s, err := valueobject.NewBatchSummary(3, total, 2, purchase, 1, reversal)
	if err != nil {
		t.Fatalf("NewBatchSummary() error = %v", err)
	}
	return s
}

func TestNewBatchOpened(t *testing.T) {
	batchID := "batch-1"
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	batchDate := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	before := time.Now().UTC()
	e := event.NewBatchOpened(batchID, terminalID, merchantID, batchDate)
	after := time.Now().UTC()

	if e.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", e.BatchID, batchID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if !e.BatchDate.Equal(batchDate) {
		t.Errorf("BatchDate = %v, want %v", e.BatchDate, batchDate)
	}
	if e.EventType() != "batch.opened" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "batch.opened")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewBatchCloseRequested(t *testing.T) {
	batchID := "batch-1"
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()

	before := time.Now().UTC()
	e := event.NewBatchCloseRequested(batchID, terminalID, merchantID)
	after := time.Now().UTC()

	if e.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", e.BatchID, batchID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if e.EventType() != "batch.close_requested" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "batch.close_requested")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewBatchClosed(t *testing.T) {
	batchID := "batch-1"
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	summary := mustSummary(t)

	before := time.Now().UTC()
	e := event.NewBatchClosed(batchID, terminalID, merchantID, summary, 2)
	after := time.Now().UTC()

	if e.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", e.BatchID, batchID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if e.Summary.TotalCount() != summary.TotalCount() {
		t.Errorf("Summary.TotalCount() = %d, want %d", e.Summary.TotalCount(), summary.TotalCount())
	}
	if e.Discrepancies != 2 {
		t.Errorf("Discrepancies = %d, want 2", e.Discrepancies)
	}
	if e.EventType() != "batch.closed" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "batch.closed")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewBatchSettled(t *testing.T) {
	batchID := "batch-1"
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	summary := mustSummary(t)

	before := time.Now().UTC()
	e := event.NewBatchSettled(batchID, terminalID, merchantID, summary)
	after := time.Now().UTC()

	if e.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", e.BatchID, batchID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if e.Summary.TotalCount() != summary.TotalCount() {
		t.Errorf("Summary.TotalCount() = %d, want %d", e.Summary.TotalCount(), summary.TotalCount())
	}
	if e.EventType() != "batch.settled" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "batch.settled")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewBatchDisputed(t *testing.T) {
	batchID := "batch-1"
	terminalID := domain.NewTerminalID()
	reason := "terminal count/amount mismatch"

	before := time.Now().UTC()
	e := event.NewBatchDisputed(batchID, terminalID, reason)
	after := time.Now().UTC()

	if e.BatchID != batchID {
		t.Errorf("BatchID = %q, want %q", e.BatchID, batchID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if e.Reason != reason {
		t.Errorf("Reason = %q, want %q", e.Reason, reason)
	}
	if e.EventType() != "batch.disputed" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "batch.disputed")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

// TestEvents_ImplementDomainEventInterface verifica que todos los eventos
// satisfacen la interfaz DomainEvent en tiempo de compilación.
func TestEvents_ImplementDomainEventInterface(t *testing.T) {
	summary := mustSummary(t)
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()

	events := []event.DomainEvent{
		event.NewBatchOpened("batch-1", terminalID, merchantID, time.Now()),
		event.NewBatchCloseRequested("batch-1", terminalID, merchantID),
		event.NewBatchClosed("batch-1", terminalID, merchantID, summary, 0),
		event.NewBatchSettled("batch-1", terminalID, merchantID, summary),
		event.NewBatchDisputed("batch-1", terminalID, "reason"),
	}

	for _, e := range events {
		if e.EventType() == "" {
			t.Errorf("%T.EventType() = \"\", want non-empty", e)
		}
		if e.OccurredAt().IsZero() {
			t.Errorf("%T.OccurredAt() is zero, want set", e)
		}
	}
}
