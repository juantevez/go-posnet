package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
)

func TestNewReceiptPayload_Success(t *testing.T) {
	r, err := valueobject.NewReceiptPayload(
		"tx-1", "Merchant Inc", "TERM-001",
		"APPROVED", 5000, "ARS", "1234", "VISA", "CHIP",
		"2026-01-01T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("NewReceiptPayload() error = %v", err)
	}
	if r.TransactionID != "tx-1" {
		t.Errorf("TransactionID = %q, want %q", r.TransactionID, "tx-1")
	}
	if r.MerchantName != "Merchant Inc" {
		t.Errorf("MerchantName = %q, want %q", r.MerchantName, "Merchant Inc")
	}
	if r.TerminalCode != "TERM-001" {
		t.Errorf("TerminalCode = %q, want %q", r.TerminalCode, "TERM-001")
	}
	if r.Result != "APPROVED" {
		t.Errorf("Result = %q, want %q", r.Result, "APPROVED")
	}
	if r.AmountCents != 5000 {
		t.Errorf("AmountCents = %d, want 5000", r.AmountCents)
	}
	if r.Currency != "ARS" {
		t.Errorf("Currency = %q, want %q", r.Currency, "ARS")
	}
	if r.CardLast4 != "1234" {
		t.Errorf("CardLast4 = %q, want %q", r.CardLast4, "1234")
	}
	if r.CardNetwork != "VISA" {
		t.Errorf("CardNetwork = %q, want %q", r.CardNetwork, "VISA")
	}
	if r.EntryMode != "CHIP" {
		t.Errorf("EntryMode = %q, want %q", r.EntryMode, "CHIP")
	}
	if r.TransactionAt != "2026-01-01T10:00:00Z" {
		t.Errorf("TransactionAt = %q, want %q", r.TransactionAt, "2026-01-01T10:00:00Z")
	}
	// Campos no seteados por el constructor — quedan en su zero value.
	if r.MerchantAddress != "" {
		t.Errorf("MerchantAddress = %q, want empty", r.MerchantAddress)
	}
	if r.AuthCode != "" {
		t.Errorf("AuthCode = %q, want empty", r.AuthCode)
	}
	if r.RejectionCode != "" {
		t.Errorf("RejectionCode = %q, want empty", r.RejectionCode)
	}
}

func TestNewReceiptPayload_ValidResults(t *testing.T) {
	for _, result := range []string{"APPROVED", "REJECTED", "REVERSED"} {
		t.Run(result, func(t *testing.T) {
			r, err := valueobject.NewReceiptPayload("tx-1", "Merchant", "TERM-001", result, 1000, "ARS", "1234", "VISA", "CHIP", "2026-01-01T10:00:00Z")
			if err != nil {
				t.Fatalf("NewReceiptPayload() error = %v", err)
			}
			if r.Result != result {
				t.Errorf("Result = %q, want %q", r.Result, result)
			}
		})
	}
}

func TestNewReceiptPayload_ValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		transactionID string
		result        string
		amountCents   int64
		wantErr       string
	}{
		{"empty transaction id", "", "APPROVED", 1000, "transaction_id cannot be empty"},
		{"invalid result", "tx-1", "PENDING", 1000, `invalid result "PENDING"`},
		{"zero amount", "tx-1", "APPROVED", 0, "amount_cents must be positive"},
		{"negative amount", "tx-1", "APPROVED", -100, "amount_cents must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := valueobject.NewReceiptPayload(tc.transactionID, "Merchant", "TERM-001", tc.result, tc.amountCents, "ARS", "1234", "VISA", "CHIP", "2026-01-01T10:00:00Z")
			if err == nil {
				t.Fatalf("NewReceiptPayload() error = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestReceiptPayload_IsApproved(t *testing.T) {
	tests := []struct {
		result string
		want   bool
	}{
		{"APPROVED", true},
		{"REJECTED", false},
		{"REVERSED", false},
	}

	for _, tc := range tests {
		t.Run(tc.result, func(t *testing.T) {
			r, err := valueobject.NewReceiptPayload("tx-1", "Merchant", "TERM-001", tc.result, 1000, "ARS", "1234", "VISA", "CHIP", "2026-01-01T10:00:00Z")
			if err != nil {
				t.Fatalf("NewReceiptPayload() error = %v", err)
			}
			if got := r.IsApproved(); got != tc.want {
				t.Errorf("IsApproved() = %v, want %v", got, tc.want)
			}
		})
	}
}
