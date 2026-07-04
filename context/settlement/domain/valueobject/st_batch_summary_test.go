package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
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

func TestNewBatchSummary_Success(t *testing.T) {
	total := mustMoney(t, 2999)
	purchase := mustMoney(t, 3000)
	reversal := mustMoney(t, 1)

	s, err := valueobject.NewBatchSummary(3, total, 2, purchase, 1, reversal)
	if err != nil {
		t.Fatalf("NewBatchSummary() error = %v", err)
	}
	if s.TotalCount() != 3 {
		t.Errorf("TotalCount() = %d, want 3", s.TotalCount())
	}
	if !s.TotalAmount().Equals(total) {
		t.Errorf("TotalAmount() = %v, want %v", s.TotalAmount(), total)
	}
	if s.PurchaseCount() != 2 {
		t.Errorf("PurchaseCount() = %d, want 2", s.PurchaseCount())
	}
	if !s.PurchaseAmount().Equals(purchase) {
		t.Errorf("PurchaseAmount() = %v, want %v", s.PurchaseAmount(), purchase)
	}
	if s.ReversalCount() != 1 {
		t.Errorf("ReversalCount() = %d, want 1", s.ReversalCount())
	}
	if !s.ReversalAmount().Equals(reversal) {
		t.Errorf("ReversalAmount() = %v, want %v", s.ReversalAmount(), reversal)
	}
	if s.IsZero() {
		t.Error("IsZero() = true, want false for a non-empty summary")
	}
}

func TestNewBatchSummary_Zero(t *testing.T) {
	s, err := valueobject.NewBatchSummary(0, domain.Money{}, 0, domain.Money{}, 0, domain.Money{})
	if err != nil {
		t.Fatalf("NewBatchSummary() error = %v", err)
	}
	if !s.IsZero() {
		t.Error("IsZero() = false, want true for an empty batch summary")
	}
}

func TestNewBatchSummary_NegativeTotalCount(t *testing.T) {
	_, err := valueobject.NewBatchSummary(-1, domain.Money{}, 0, domain.Money{}, 0, domain.Money{})
	if err == nil || !strings.Contains(err.Error(), "cannot be negative") {
		t.Fatalf("error = %v, want it to mention total_count cannot be negative", err)
	}
}

func TestNewBatchSummary_CountMismatch(t *testing.T) {
	_, err := valueobject.NewBatchSummary(5, domain.Money{}, 2, domain.Money{}, 2, domain.Money{})
	if err == nil || !strings.Contains(err.Error(), "purchase_count") {
		t.Fatalf("error = %v, want it to mention the count mismatch", err)
	}
}
