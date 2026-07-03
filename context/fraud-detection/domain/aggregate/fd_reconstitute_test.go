package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

func TestReconstitute_CopiesAllFields(t *testing.T) {
	id := "fraud-case-123"
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	occurredAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	evaluatedAt := occurredAt.Add(time.Second)
	score, err := valueobject.NewFraudScore(30, []string{"R1"})
	if err != nil {
		t.Fatalf("NewFraudScore() error = %v", err)
	}
	evals := []valueobject.RuleEvaluation{mustRuleEval(t, "R1", true, 30, "reason")}

	fc := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            id,
		TransactionID: txID,
		TerminalID:    terminalID,
		MerchantID:    merchantID,
		AmountCents:   5000,
		Currency:      "ARS",
		CardNetwork:   "VISA",
		EntryMode:     "CHIP",
		OccurredAt:    occurredAt,
		Score:         score,
		Evaluations:   evals,
		EvaluatedAt:   &evaluatedAt,
	})

	if fc.ID() != id {
		t.Errorf("ID() = %q, want %q", fc.ID(), id)
	}
	if !fc.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", fc.TransactionID(), txID)
	}
	if !fc.TerminalID().Equals(terminalID) {
		t.Errorf("TerminalID() = %v, want %v", fc.TerminalID(), terminalID)
	}
	if !fc.MerchantID().Equals(merchantID) {
		t.Errorf("MerchantID() = %v, want %v", fc.MerchantID(), merchantID)
	}
	if fc.AmountCents() != 5000 {
		t.Errorf("AmountCents() = %d, want 5000", fc.AmountCents())
	}
	if fc.Currency() != "ARS" {
		t.Errorf("Currency() = %q, want %q", fc.Currency(), "ARS")
	}
	if fc.CardNetwork() != "VISA" {
		t.Errorf("CardNetwork() = %q, want %q", fc.CardNetwork(), "VISA")
	}
	if fc.EntryMode() != "CHIP" {
		t.Errorf("EntryMode() = %q, want %q", fc.EntryMode(), "CHIP")
	}
	if !fc.OccurredAt().Equal(occurredAt) {
		t.Errorf("OccurredAt() = %v, want %v", fc.OccurredAt(), occurredAt)
	}
	if fc.Score().Score() != 30 {
		t.Errorf("Score().Score() = %d, want 30", fc.Score().Score())
	}
	if len(fc.Evaluations()) != 1 || fc.Evaluations()[0].RuleID() != "R1" {
		t.Errorf("Evaluations() = %v, want [R1]", fc.Evaluations())
	}
	if fc.EvaluatedAt() == nil || !fc.EvaluatedAt().Equal(evaluatedAt) {
		t.Errorf("EvaluatedAt() = %v, want %v", fc.EvaluatedAt(), evaluatedAt)
	}

	// Reconstitute siempre marca el caso como evaluado, independientemente
	// de si Score/Evaluations vinieron vacíos.
	if !fc.IsEvaluated() {
		t.Error("IsEvaluated() = false, want true")
	}

	// Reconstitute no debe emitir eventos de dominio.
	if len(fc.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() = %v, want empty", fc.DomainEvents())
	}
}

func TestReconstitute_NilEvaluatedAt(t *testing.T) {
	fc := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "fraud-case-456",
		TransactionID: domain.NewTransactionID(),
		TerminalID:    domain.NewTerminalID(),
		MerchantID:    domain.NewMerchantID(),
		AmountCents:   1000,
		Currency:      "ARS",
		CardNetwork:   "VISA",
		EntryMode:     "CHIP",
		OccurredAt:    time.Now().UTC(),
	})

	if fc.EvaluatedAt() != nil {
		t.Errorf("EvaluatedAt() = %v, want nil", fc.EvaluatedAt())
	}
	if !fc.IsEvaluated() {
		t.Error("IsEvaluated() = false, want true")
	}
	if !fc.Score().IsZero() {
		t.Errorf("Score() = %+v, want zero value when not provided", fc.Score())
	}
	if fc.Evaluations() != nil {
		t.Errorf("Evaluations() = %v, want nil when not provided", fc.Evaluations())
	}
}
