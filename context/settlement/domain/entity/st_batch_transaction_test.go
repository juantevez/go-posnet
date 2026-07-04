package entity_test

import (
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/entity"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func TestNewBatchTransaction_Success(t *testing.T) {
	txID := domain.NewTransactionID()

	tx, err := entity.NewBatchTransaction("batch-1", txID, 1000, "ARS", valueobject.BatchTxPurchase)
	if err != nil {
		t.Fatalf("NewBatchTransaction() error = %v", err)
	}

	if tx.ID() == "" {
		t.Error("ID() is empty, want a generated UUID")
	}
	if tx.BatchID() != "batch-1" {
		t.Errorf("BatchID() = %q, want %q", tx.BatchID(), "batch-1")
	}
	if !tx.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", tx.TransactionID(), txID)
	}
	if tx.AmountCents() != 1000 {
		t.Errorf("AmountCents() = %d, want 1000", tx.AmountCents())
	}
	if tx.Currency() != "ARS" {
		t.Errorf("Currency() = %q, want %q", tx.Currency(), "ARS")
	}
	if tx.Type() != valueobject.BatchTxPurchase {
		t.Errorf("Type() = %v, want %v", tx.Type(), valueobject.BatchTxPurchase)
	}
	if time.Since(tx.IncludedAt()) > 5*time.Second {
		t.Errorf("IncludedAt() = %v, want close to now", tx.IncludedAt())
	}
}

func TestNewBatchTransaction_GeneratesUniqueIDs(t *testing.T) {
	tx1, err := entity.NewBatchTransaction("batch-1", domain.NewTransactionID(), 1000, "ARS", valueobject.BatchTxPurchase)
	if err != nil {
		t.Fatalf("NewBatchTransaction() error = %v", err)
	}
	tx2, err := entity.NewBatchTransaction("batch-1", domain.NewTransactionID(), 1000, "ARS", valueobject.BatchTxPurchase)
	if err != nil {
		t.Fatalf("NewBatchTransaction() error = %v", err)
	}
	if tx1.ID() == tx2.ID() {
		t.Error("two calls to NewBatchTransaction() produced the same ID")
	}
}

func TestNewBatchTransaction_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		batchID     string
		txID        domain.TransactionID
		amountCents int64
		wantSubstr  string
	}{
		{"empty batch_id", "", domain.NewTransactionID(), 1000, "batch_id"},
		{"zero transaction_id", "batch-1", domain.TransactionID{}, 1000, "transaction_id"},
		{"zero amount", "batch-1", domain.NewTransactionID(), 0, "amount_cents"},
		{"negative amount", "batch-1", domain.NewTransactionID(), -1, "amount_cents"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := entity.NewBatchTransaction(tc.batchID, tc.txID, tc.amountCents, "ARS", valueobject.BatchTxPurchase)
			if err == nil || !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantSubstr)
			}
		})
	}
}

func TestReconstituteBatchTransaction(t *testing.T) {
	txID := domain.NewTransactionID()
	includedAt := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	tx := entity.ReconstituteBatchTransaction(
		"tx-1", "batch-1", txID, 1000, "ARS", valueobject.BatchTxReversal, includedAt,
	)

	if tx.ID() != "tx-1" {
		t.Errorf("ID() = %q, want %q", tx.ID(), "tx-1")
	}
	if tx.BatchID() != "batch-1" {
		t.Errorf("BatchID() = %q, want %q", tx.BatchID(), "batch-1")
	}
	if !tx.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", tx.TransactionID(), txID)
	}
	if tx.AmountCents() != 1000 {
		t.Errorf("AmountCents() = %d, want 1000", tx.AmountCents())
	}
	if tx.Currency() != "ARS" {
		t.Errorf("Currency() = %q, want %q", tx.Currency(), "ARS")
	}
	if tx.Type() != valueobject.BatchTxReversal {
		t.Errorf("Type() = %v, want %v", tx.Type(), valueobject.BatchTxReversal)
	}
	if !tx.IncludedAt().Equal(includedAt) {
		t.Errorf("IncludedAt() = %v, want %v", tx.IncludedAt(), includedAt)
	}
}

// ReconstituteBatchTransaction no valida invariantes — refleja fielmente
// datos ya persistidos, incluso si técnicamente violarían NewBatchTransaction.
func TestReconstituteBatchTransaction_DoesNotValidate(t *testing.T) {
	tx := entity.ReconstituteBatchTransaction(
		"tx-1", "", domain.TransactionID{}, -500, "", valueobject.BatchTransactionType("BOGUS"), time.Time{},
	)

	if tx.BatchID() != "" {
		t.Errorf("BatchID() = %q, want empty (no validation on Reconstitute)", tx.BatchID())
	}
	if tx.AmountCents() != -500 {
		t.Errorf("AmountCents() = %d, want -500 (no validation on Reconstitute)", tx.AmountCents())
	}
}
