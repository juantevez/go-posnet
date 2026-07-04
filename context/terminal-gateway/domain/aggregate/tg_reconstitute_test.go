package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func TestReconstitute(t *testing.T) {
	id := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	amount := mustMoney(t)
	stan := mustSTAN(t)
	expiresAt := time.Date(2026, 1, 15, 12, 5, 0, 0, time.UTC)
	createdAt := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	closedAt := time.Date(2026, 1, 15, 12, 2, 0, 0, time.UTC)

	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:              id,
		TerminalID:      terminalID,
		MerchantID:      merchantID,
		Amount:          amount,
		STAN:            stan,
		Channel:         valueobject.ChannelNFC,
		State:           valueobject.StateRejected,
		AuthCode:        "",
		RejectionCode:   "05",
		RejectionReason: "Do not honor",
		ExpiresAt:       expiresAt,
		CreatedAt:       createdAt,
		ClosedAt:        &closedAt,
	})

	if s.ID() != id {
		t.Errorf("ID() = %v, want %v", s.ID(), id)
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
	if s.Channel() != valueobject.ChannelNFC {
		t.Errorf("Channel() = %v, want %v", s.Channel(), valueobject.ChannelNFC)
	}
	if s.State() != valueobject.StateRejected {
		t.Errorf("State() = %v, want %v", s.State(), valueobject.StateRejected)
	}
	if s.RejectionCode() != "05" {
		t.Errorf("RejectionCode() = %q, want %q", s.RejectionCode(), "05")
	}
	if s.RejectionReason() != "Do not honor" {
		t.Errorf("RejectionReason() = %q, want %q", s.RejectionReason(), "Do not honor")
	}
	if !s.ExpiresAt().Equal(expiresAt) {
		t.Errorf("ExpiresAt() = %v, want %v", s.ExpiresAt(), expiresAt)
	}
	if !s.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", s.CreatedAt(), createdAt)
	}
	if s.ClosedAt() == nil || !s.ClosedAt().Equal(closedAt) {
		t.Errorf("ClosedAt() = %v, want %v", s.ClosedAt(), closedAt)
	}
	if len(s.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() len = %d, want 0 (Reconstitute no emite eventos)", len(s.DomainEvents()))
	}
}

func TestReconstitute_NilClosedAt(t *testing.T) {
	s := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelQR,
		State:      valueobject.StateAwaitingPayment,
		ExpiresAt:  time.Now().UTC(),
		CreatedAt:  time.Now().UTC(),
	})

	if s.ClosedAt() != nil {
		t.Errorf("ClosedAt() = %v, want nil", s.ClosedAt())
	}
	if s.AuthCode() != "" {
		t.Errorf("AuthCode() = %q, want empty", s.AuthCode())
	}
}
