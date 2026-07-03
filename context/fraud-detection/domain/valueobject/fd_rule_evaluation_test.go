package valueobject_test

import (
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
)

func TestNewRuleEvaluation_Success(t *testing.T) {
	e, err := valueobject.NewRuleEvaluation("R1", "High Amount", true, 30, "amount above average")
	if err != nil {
		t.Fatalf("NewRuleEvaluation() error = %v", err)
	}
	if e.RuleID() != "R1" {
		t.Errorf("RuleID() = %q, want %q", e.RuleID(), "R1")
	}
	if e.RuleName() != "High Amount" {
		t.Errorf("RuleName() = %q, want %q", e.RuleName(), "High Amount")
	}
	if !e.Activated() {
		t.Error("Activated() = false, want true")
	}
	if e.ScoreContribution() != 30 {
		t.Errorf("ScoreContribution() = %d, want 30", e.ScoreContribution())
	}
	if e.Reason() != "amount above average" {
		t.Errorf("Reason() = %q, want %q", e.Reason(), "amount above average")
	}
}

func TestNewRuleEvaluation_NotActivatedWithZeroScore(t *testing.T) {
	e, err := valueobject.NewRuleEvaluation("R2", "Unused Rule", false, 0, "")
	if err != nil {
		t.Fatalf("NewRuleEvaluation() error = %v", err)
	}
	if e.Activated() {
		t.Error("Activated() = true, want false")
	}
	if e.ScoreContribution() != 0 {
		t.Errorf("ScoreContribution() = %d, want 0", e.ScoreContribution())
	}
}

func TestNewRuleEvaluation_BoundaryScoreContribution(t *testing.T) {
	t.Run("100 with activated", func(t *testing.T) {
		if _, err := valueobject.NewRuleEvaluation("R1", "Rule", true, 100, "reason"); err != nil {
			t.Errorf("NewRuleEvaluation(..., 100, ...) error = %v, want nil", err)
		}
	})
	t.Run("0 with not activated", func(t *testing.T) {
		if _, err := valueobject.NewRuleEvaluation("R1", "Rule", false, 0, ""); err != nil {
			t.Errorf("NewRuleEvaluation(..., 0, ...) error = %v, want nil", err)
		}
	})
}

func TestNewRuleEvaluation_ValidationErrors(t *testing.T) {
	tests := []struct {
		name              string
		ruleID            string
		activated         bool
		scoreContribution int
		wantErr           string
	}{
		{"empty rule id", "", true, 30, "rule_id cannot be empty"},
		{"negative score", "R1", true, -1, "out of range"},
		{"score above 100", "R1", true, 101, "out of range"},
		{"activated with zero score", "R1", true, 0, "activated rule must have score_contribution > 0"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := valueobject.NewRuleEvaluation(tc.ruleID, "Rule", tc.activated, tc.scoreContribution, "reason")
			if err == nil {
				t.Fatalf("NewRuleEvaluation() error = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}
