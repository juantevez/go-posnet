package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/entity"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func TestReconstitute_CopiesAllFields(t *testing.T) {
	id := "notif-123"
	txID := domain.NewTransactionID()
	merchantID := domain.NewMerchantID()
	receipt := mustReceipt(t)
	createdAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	dispatchedAt := createdAt.Add(time.Minute)
	nextRetryAt := createdAt.Add(30 * time.Second)
	attempt := entity.ReconstituteDeliveryAttempt("notif-123-1", id, 1, true, 200, "", createdAt)

	n := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            id,
		TransactionID: txID,
		MerchantID:    merchantID,
		Channel:       valueobject.ChannelEmail,
		State:         valueobject.StateSent,
		Receipt:       receipt,
		Attempts:      []*entity.DeliveryAttempt{attempt},
		MaxAttempts:   7,
		CreatedAt:     createdAt,
		DispatchedAt:  &dispatchedAt,
		NextRetryAt:   &nextRetryAt,
	})

	if n.ID() != id {
		t.Errorf("ID() = %q, want %q", n.ID(), id)
	}
	if !n.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", n.TransactionID(), txID)
	}
	if !n.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", n.MerchantID(), merchantID)
	}
	if n.Channel() != valueobject.ChannelEmail {
		t.Errorf("Channel() = %v, want %v", n.Channel(), valueobject.ChannelEmail)
	}
	if n.State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StateSent)
	}
	if n.Receipt() != receipt {
		t.Errorf("Receipt() = %v, want %v", n.Receipt(), receipt)
	}
	if len(n.Attempts()) != 1 || n.Attempts()[0] != attempt {
		t.Errorf("Attempts() = %v, want [%v]", n.Attempts(), attempt)
	}
	if n.AttemptCount() != 1 {
		t.Errorf("AttemptCount() = %d, want 1", n.AttemptCount())
	}
	if n.MaxAttempts() != 7 {
		t.Errorf("MaxAttempts() = %d, want 7", n.MaxAttempts())
	}
	if !n.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", n.CreatedAt(), createdAt)
	}
	if n.DispatchedAt() == nil || !n.DispatchedAt().Equal(dispatchedAt) {
		t.Errorf("DispatchedAt() = %v, want %v", n.DispatchedAt(), dispatchedAt)
	}
	if n.NextRetryAt() == nil || !n.NextRetryAt().Equal(nextRetryAt) {
		t.Errorf("NextRetryAt() = %v, want %v", n.NextRetryAt(), nextRetryAt)
	}

	// Reconstitute no debe emitir eventos de dominio.
	if len(n.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() = %v, want empty", n.DomainEvents())
	}
}

func TestReconstitute_NilOptionalFields(t *testing.T) {
	n := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "notif-456",
		TransactionID: domain.NewTransactionID(),
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelSMS,
		State:         valueobject.StatePending,
		Receipt:       mustReceipt(t),
		MaxAttempts:   5,
		CreatedAt:     time.Now().UTC(),
	})

	if n.DispatchedAt() != nil {
		t.Errorf("DispatchedAt() = %v, want nil", n.DispatchedAt())
	}
	if n.NextRetryAt() != nil {
		t.Errorf("NextRetryAt() = %v, want nil", n.NextRetryAt())
	}
	if n.Attempts() != nil {
		t.Errorf("Attempts() = %v, want nil", n.Attempts())
	}
	if n.AttemptCount() != 0 {
		t.Errorf("AttemptCount() = %d, want 0", n.AttemptCount())
	}
}
