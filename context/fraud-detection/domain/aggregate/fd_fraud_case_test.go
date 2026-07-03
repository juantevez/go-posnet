package aggregate_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/event"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func mustRuleEval(t *testing.T, ruleID string, activated bool, score int, reason string) valueobject.RuleEvaluation {
	t.Helper()
	e, err := valueobject.NewRuleEvaluation(ruleID, "Rule "+ruleID, activated, score, reason)
	if err != nil {
		t.Fatalf("NewRuleEvaluation(%q) error = %v", ruleID, err)
	}
	return e
}

func newValidFraudCase(t *testing.T) *aggregate.FraudCase {
	t.Helper()
	fc, err := aggregate.NewFraudCase(
		domain.NewTransactionID(),
		domain.NewTerminalID(),
		domain.NewMerchantID(),
		5000,
		"ARS",
		"VISA",
		"CHIP",
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("NewFraudCase() error = %v", err)
	}
	return fc
}

// ─── NewFraudCase ───────────────────────────────────────────────────────────

func TestNewFraudCase_Success(t *testing.T) {
	txID := domain.NewTransactionID()
	terminalID := domain.NewTerminalID()
	merchantID := domain.NewMerchantID()
	occurredAt := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	fc, err := aggregate.NewFraudCase(txID, terminalID, merchantID, 5000, "ARS", "VISA", "CHIP", occurredAt)
	if err != nil {
		t.Fatalf("NewFraudCase() error = %v", err)
	}

	if fc.ID() == "" {
		t.Error("ID() is empty, want a generated UUID")
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
	if fc.IsEvaluated() {
		t.Error("IsEvaluated() = true, want false")
	}
	if fc.EvaluatedAt() != nil {
		t.Errorf("EvaluatedAt() = %v, want nil", fc.EvaluatedAt())
	}
	if fc.Evaluations() != nil {
		t.Errorf("Evaluations() = %v, want nil", fc.Evaluations())
	}
	if !fc.Score().IsZero() {
		t.Errorf("Score() = %+v, want zero value", fc.Score())
	}
	if len(fc.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() = %v, want empty", fc.DomainEvents())
	}
}

func TestNewFraudCase_ValidationErrors(t *testing.T) {
	occurredAt := time.Now().UTC()

	t.Run("zero transaction id", func(t *testing.T) {
		fc, err := aggregate.NewFraudCase(domain.TransactionID{}, domain.NewTerminalID(), domain.NewMerchantID(), 5000, "ARS", "VISA", "CHIP", occurredAt)
		if err == nil {
			t.Fatal("NewFraudCase() error = nil, want error")
		}
		if fc != nil {
			t.Errorf("NewFraudCase() fc = %v, want nil", fc)
		}
	})

	t.Run("zero amount", func(t *testing.T) {
		_, err := aggregate.NewFraudCase(domain.NewTransactionID(), domain.NewTerminalID(), domain.NewMerchantID(), 0, "ARS", "VISA", "CHIP", occurredAt)
		if err == nil {
			t.Fatal("NewFraudCase() error = nil, want error")
		}
	})

	t.Run("negative amount", func(t *testing.T) {
		_, err := aggregate.NewFraudCase(domain.NewTransactionID(), domain.NewTerminalID(), domain.NewMerchantID(), -100, "ARS", "VISA", "CHIP", occurredAt)
		if err == nil {
			t.Fatal("NewFraudCase() error = nil, want error")
		}
	})
}

// ─── ApplyEvaluations ─────────────────────────────────────────────────────────

func TestApplyEvaluations_Success(t *testing.T) {
	fc := newValidFraudCase(t)
	evals := []valueobject.RuleEvaluation{
		mustRuleEval(t, "R1", true, 30, "amount above average"),
		mustRuleEval(t, "R2", false, 0, ""),
	}

	before := time.Now().UTC()
	if err := fc.ApplyEvaluations(evals); err != nil {
		t.Fatalf("ApplyEvaluations() error = %v", err)
	}
	after := time.Now().UTC()

	if !fc.IsEvaluated() {
		t.Error("IsEvaluated() = false, want true")
	}
	if len(fc.Evaluations()) != 2 {
		t.Errorf("Evaluations() = %v, want 2 items", fc.Evaluations())
	}
	if fc.Score().Score() != 30 {
		t.Errorf("Score().Score() = %d, want 30", fc.Score().Score())
	}
	if fc.Score().Decision() != valueobject.DecisionApprove {
		t.Errorf("Score().Decision() = %v, want %v", fc.Score().Decision(), valueobject.DecisionApprove)
	}
	if len(fc.Score().RulesHit()) != 1 || fc.Score().RulesHit()[0] != "R1" {
		t.Errorf("Score().RulesHit() = %v, want [R1]", fc.Score().RulesHit())
	}

	evaluatedAt := fc.EvaluatedAt()
	if evaluatedAt == nil {
		t.Fatal("EvaluatedAt() = nil, want non-nil")
	}
	if evaluatedAt.Before(before) || evaluatedAt.After(after) {
		t.Errorf("EvaluatedAt() = %v, want between %v and %v", evaluatedAt, before, after)
	}

	events := fc.DomainEvents()
	if len(events) != 1 {
		t.Fatalf("DomainEvents() = %d, want 1", len(events))
	}
	evaluated, ok := events[0].(event.FraudCaseEvaluated)
	if !ok {
		t.Fatalf("DomainEvents()[0] type = %T, want event.FraudCaseEvaluated", events[0])
	}
	if evaluated.FraudCaseID != fc.ID() {
		t.Errorf("FraudCaseEvaluated.FraudCaseID = %q, want %q", evaluated.FraudCaseID, fc.ID())
	}
	if !evaluated.TransactionID.Equals(fc.TransactionID()) {
		t.Errorf("FraudCaseEvaluated.TransactionID = %v, want %v", evaluated.TransactionID, fc.TransactionID())
	}
	if evaluated.Score.Score() != 30 {
		t.Errorf("FraudCaseEvaluated.Score.Score() = %d, want 30", evaluated.Score.Score())
	}
}

func TestApplyEvaluations_ScoreClampedTo100(t *testing.T) {
	fc := newValidFraudCase(t)
	evals := []valueobject.RuleEvaluation{
		mustRuleEval(t, "R1", true, 70, "reason 1"),
		mustRuleEval(t, "R2", true, 70, "reason 2"),
	}

	if err := fc.ApplyEvaluations(evals); err != nil {
		t.Fatalf("ApplyEvaluations() error = %v", err)
	}
	if fc.Score().Score() != 100 {
		t.Errorf("Score().Score() = %d, want 100 (clamped)", fc.Score().Score())
	}
	if fc.Score().Decision() != valueobject.DecisionReject {
		t.Errorf("Score().Decision() = %v, want %v", fc.Score().Decision(), valueobject.DecisionReject)
	}
	if len(fc.Score().RulesHit()) != 2 {
		t.Errorf("Score().RulesHit() = %v, want 2 items", fc.Score().RulesHit())
	}
}

func TestApplyEvaluations_AlreadyEvaluated(t *testing.T) {
	fc := newValidFraudCase(t)
	evals := []valueobject.RuleEvaluation{mustRuleEval(t, "R1", true, 10, "reason")}

	if err := fc.ApplyEvaluations(evals); err != nil {
		t.Fatalf("first ApplyEvaluations() error = %v", err)
	}
	err := fc.ApplyEvaluations(evals)
	if err == nil {
		t.Fatal("second ApplyEvaluations() error = nil, want error")
	}
}

func TestApplyEvaluations_EmptyEvaluations(t *testing.T) {
	fc := newValidFraudCase(t)
	err := fc.ApplyEvaluations([]valueobject.RuleEvaluation{})
	if err == nil {
		t.Fatal("ApplyEvaluations() error = nil, want error")
	}
	if fc.IsEvaluated() {
		t.Error("IsEvaluated() = true, want false after failed ApplyEvaluations")
	}
}

// ─── DomainEvents / ClearDomainEvents ─────────────────────────────────────────

func TestFraudCase_ClearDomainEvents(t *testing.T) {
	fc := newValidFraudCase(t)
	evals := []valueobject.RuleEvaluation{mustRuleEval(t, "R1", true, 10, "reason")}
	if err := fc.ApplyEvaluations(evals); err != nil {
		t.Fatalf("ApplyEvaluations() error = %v", err)
	}
	if len(fc.DomainEvents()) == 0 {
		t.Fatal("DomainEvents() = empty, want at least the evaluated event")
	}
	fc.ClearDomainEvents()
	if len(fc.DomainEvents()) != 0 {
		t.Errorf("DomainEvents() after ClearDomainEvents() = %v, want empty", fc.DomainEvents())
	}
}
