package aggregate_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
)

func TestBuildNeutralScore_Success(t *testing.T) {
	fc := newValidFraudCase(t)

	score, err := fc.BuildNeutralScore("engine timeout")
	if err != nil {
		t.Fatalf("BuildNeutralScore() error = %v", err)
	}
	if score.Score() != 50 {
		t.Errorf("Score() = %d, want 50", score.Score())
	}
	if score.Decision() != valueobject.DecisionReview {
		t.Errorf("Decision() = %v, want %v", score.Decision(), valueobject.DecisionReview)
	}
	if len(score.RulesHit()) != 1 || score.RulesHit()[0] != "BYPASS" {
		t.Errorf("RulesHit() = %v, want [BYPASS]", score.RulesHit())
	}

	if !fc.IsEvaluated() {
		t.Error("IsEvaluated() = false, want true")
	}
	if len(fc.Evaluations()) != 1 {
		t.Fatalf("Evaluations() = %v, want 1 item", fc.Evaluations())
	}
	eval := fc.Evaluations()[0]
	if eval.RuleID() != "BYPASS" {
		t.Errorf("Evaluations()[0].RuleID() = %q, want %q", eval.RuleID(), "BYPASS")
	}
	if !strings.Contains(eval.Reason(), "engine timeout") {
		t.Errorf("Evaluations()[0].Reason() = %q, want it to contain %q", eval.Reason(), "engine timeout")
	}
	if len(fc.DomainEvents()) != 1 {
		t.Errorf("DomainEvents() = %d, want 1", len(fc.DomainEvents()))
	}
}

func TestBuildNeutralScore_AlreadyEvaluated(t *testing.T) {
	fc := newValidFraudCase(t)
	evals := []valueobject.RuleEvaluation{mustRuleEval(t, "R1", true, 10, "reason")}
	if err := fc.ApplyEvaluations(evals); err != nil {
		t.Fatalf("ApplyEvaluations() error = %v", err)
	}

	score, err := fc.BuildNeutralScore("engine timeout")
	if err == nil {
		t.Fatal("BuildNeutralScore() error = nil, want error")
	}
	if !score.IsZero() {
		t.Errorf("BuildNeutralScore() score = %+v, want zero value", score)
	}
	// El resultado de la primera evaluación no debe verse alterado.
	if fc.Score().Score() != 10 {
		t.Errorf("Score().Score() = %d, want unchanged 10", fc.Score().Score())
	}
}
