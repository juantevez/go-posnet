package command_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

func TestUpdateRuleThreshold_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cmd     port.UpdateRuleThresholdCommand
		wantErr string
	}{
		{"empty rule id", port.UpdateRuleThresholdCommand{RuleID: "", NewScoreWeight: 50}, "rule_id cannot be empty"},
		{"score weight zero", port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: 0}, "new_score_weight must be between 1 and 100"},
		{"score weight negative", port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: -5}, "new_score_weight must be between 1 and 100"},
		{"score weight above 100", port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: 101}, "new_score_weight must be between 1 and 100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := command.NewAdminHandler(&fakeRuleRepo{}, &fakeFraudCaseRepo{})
			err := h.UpdateRuleThreshold(context.Background(), tc.cmd)
			var ve *pkgerrors.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestUpdateRuleThreshold_BoundaryScoreWeightIsValid(t *testing.T) {
	for _, weight := range []int{1, 100} {
		t.Run(fmt.Sprintf("weight=%d", weight), func(t *testing.T) {
			rule := mustFraudRule(t, "R1", 10)
			ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{rule}}
			h := command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{})

			err := h.UpdateRuleThreshold(context.Background(), port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: weight})
			if err != nil {
				t.Errorf("UpdateRuleThreshold() error = %v, want nil", err)
			}
		})
	}
}

func TestUpdateRuleThreshold_LoadRulesError(t *testing.T) {
	ruleRepo := &fakeRuleRepo{findErr: errors.New("connection reset")}
	h := command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{})

	err := h.UpdateRuleThreshold(context.Background(), port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: 50})
	if err == nil || !strings.Contains(err.Error(), "load rules") {
		t.Fatalf("error = %v, want it to contain %q", err, "load rules")
	}
}

func TestUpdateRuleThreshold_RuleNotFound(t *testing.T) {
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{mustFraudRule(t, "R1", 10)}}
	h := command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{})

	err := h.UpdateRuleThreshold(context.Background(), port.UpdateRuleThresholdCommand{RuleID: "R999", NewScoreWeight: 50})
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestUpdateRuleThreshold_Success(t *testing.T) {
	rule := mustFraudRule(t, "R1", 10)
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{rule}}
	h := command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{})

	err := h.UpdateRuleThreshold(context.Background(), port.UpdateRuleThresholdCommand{
		RuleID:         "R1",
		NewThreshold:   99,
		NewScoreWeight: 50,
	})
	if err != nil {
		t.Fatalf("UpdateRuleThreshold() error = %v", err)
	}
	if len(ruleRepo.savedRules) != 1 {
		t.Fatalf("saved rules = %d, want 1", len(ruleRepo.savedRules))
	}
	// Nota: la implementación actual tiene un TODO explícito ("En la
	// implementación completa: rule.UpdateThreshold(...)") — NewThreshold y
	// NewScoreWeight no se aplican todavía a la regla, se guarda tal cual
	// estaba. Este test documenta el comportamiento actual, no el deseado.
	if ruleRepo.savedRules[0].ScoreWeight() != 10 {
		t.Errorf("savedRules[0].ScoreWeight() = %d, want 10 (sin cambios — ver TODO en UpdateRuleThreshold)", ruleRepo.savedRules[0].ScoreWeight())
	}
}

func TestUpdateRuleThreshold_SaveError(t *testing.T) {
	rule := mustFraudRule(t, "R1", 10)
	ruleRepo := &fakeRuleRepo{rules: []*entity.FraudRule{rule}, saveErr: errors.New("connection reset")}
	h := command.NewAdminHandler(ruleRepo, &fakeFraudCaseRepo{})

	err := h.UpdateRuleThreshold(context.Background(), port.UpdateRuleThresholdCommand{RuleID: "R1", NewScoreWeight: 50})
	if err == nil || !strings.Contains(err.Error(), "save rule") {
		t.Fatalf("error = %v, want it to contain %q", err, "save rule")
	}
}
