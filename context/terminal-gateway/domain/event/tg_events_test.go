package event_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/event"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// withinWindow verifica que ts esté entre before y after, inclusive.
func withinWindow(t *testing.T, ts, before, after time.Time) {
	t.Helper()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp = %v, want between %v and %v", ts, before, after)
	}
}

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

func TestNewSessionCreated(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	amount := mustMoney(t)
	stan := mustSTAN(t)
	expiresAt := time.Now().UTC().Add(5 * time.Minute)

	before := time.Now().UTC()
	e := event.NewSessionCreated(txID, terminalID, merchantID, amount, stan, valueobject.ChannelQR, expiresAt)
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if !e.Amount.Equals(amount) {
		t.Errorf("Amount = %v, want %v", e.Amount, amount)
	}
	if e.STAN != stan {
		t.Errorf("STAN = %v, want %v", e.STAN, stan)
	}
	if e.Channel != valueobject.ChannelQR {
		t.Errorf("Channel = %v, want %v", e.Channel, valueobject.ChannelQR)
	}
	if !e.ExpiresAt.Equal(expiresAt) {
		t.Errorf("ExpiresAt = %v, want %v", e.ExpiresAt, expiresAt)
	}
	if e.EventType() != "session.created" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "session.created")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewTransactionInitiated(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	amount := mustMoney(t)
	stan := mustSTAN(t)
	iso8583Raw := []byte("iso8583-raw-bytes")

	before := time.Now().UTC()
	e := event.NewTransactionInitiated(txID, terminalID, merchantID, amount, stan, valueobject.ChannelNFC, iso8583Raw, "emv-base64")
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if !e.MerchantID.Equals(merchantID) {
		t.Errorf("MerchantID = %v, want %v", e.MerchantID, merchantID)
	}
	if !e.Amount.Equals(amount) {
		t.Errorf("Amount = %v, want %v", e.Amount, amount)
	}
	if e.STAN != stan {
		t.Errorf("STAN = %v, want %v", e.STAN, stan)
	}
	if e.Channel != valueobject.ChannelNFC {
		t.Errorf("Channel = %v, want %v", e.Channel, valueobject.ChannelNFC)
	}
	if string(e.ISO8583Raw) != string(iso8583Raw) {
		t.Errorf("ISO8583Raw = %q, want %q", e.ISO8583Raw, iso8583Raw)
	}
	if e.EMVDataBase64 != "emv-base64" {
		t.Errorf("EMVDataBase64 = %q, want %q", e.EMVDataBase64, "emv-base64")
	}
	if e.EventType() != "transaction.initiated" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "transaction.initiated")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewSessionApproved(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()

	before := time.Now().UTC()
	e := event.NewSessionApproved(txID, terminalID, "AUTH123")
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if e.AuthCode != "AUTH123" {
		t.Errorf("AuthCode = %q, want %q", e.AuthCode, "AUTH123")
	}
	if e.EventType() != "session.approved" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "session.approved")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewSessionRejected(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()

	before := time.Now().UTC()
	e := event.NewSessionRejected(txID, terminalID, "05", "Do not honor")
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if e.RejectionCode != "05" {
		t.Errorf("RejectionCode = %q, want %q", e.RejectionCode, "05")
	}
	if e.RejectionReason != "Do not honor" {
		t.Errorf("RejectionReason = %q, want %q", e.RejectionReason, "Do not honor")
	}
	if e.EventType() != "session.rejected" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "session.rejected")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewSessionExpired(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()

	before := time.Now().UTC()
	e := event.NewSessionExpired(txID, terminalID)
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if e.EventType() != "session.expired" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "session.expired")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewSessionCancelled(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()

	before := time.Now().UTC()
	e := event.NewSessionCancelled(txID, terminalID)
	after := time.Now().UTC()

	if e.TransactionID != txID {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if !e.TerminalID.Equals(terminalID) {
		t.Errorf("TerminalID = %v, want %v", e.TerminalID, terminalID)
	}
	if e.EventType() != "session.cancelled" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "session.cancelled")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

// TestEvents_ImplementDomainEventInterface verifica que todos los eventos
// satisfacen la interfaz DomainEvent en tiempo de compilación.
func TestEvents_ImplementDomainEventInterface(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()

	events := []event.DomainEvent{
		event.NewSessionCreated(txID, terminalID, merchantID, mustMoney(t), mustSTAN(t), valueobject.ChannelQR, time.Now()),
		event.NewTransactionInitiated(txID, terminalID, merchantID, mustMoney(t), mustSTAN(t), valueobject.ChannelNFC, nil, ""),
		event.NewSessionApproved(txID, terminalID, "AUTH123"),
		event.NewSessionRejected(txID, terminalID, "05", "Do not honor"),
		event.NewSessionExpired(txID, terminalID),
		event.NewSessionCancelled(txID, terminalID),
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
