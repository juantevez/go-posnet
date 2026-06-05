package valueobject

import "fmt"

// RuleEvaluation registra el resultado de aplicar una regla de fraude
// a una transacción específica. Es inmutable.
type RuleEvaluation struct {
	ruleID            string
	ruleName          string
	activated         bool   // ¿La regla se disparó?
	scoreContribution int    // Puntos sumados al score total (0 si no activó)
	reason            string // Descripción de por qué activó (para auditoría)
}

// NewRuleEvaluation crea una evaluación de regla.
func NewRuleEvaluation(ruleID, ruleName string, activated bool, scoreContribution int, reason string) (RuleEvaluation, error) {
	if ruleID == "" {
		return RuleEvaluation{}, fmt.Errorf("rule_evaluation: rule_id cannot be empty")
	}
	if scoreContribution < 0 || scoreContribution > 100 {
		return RuleEvaluation{}, fmt.Errorf("rule_evaluation: score_contribution %d out of range [0, 100]", scoreContribution)
	}
	if activated && scoreContribution == 0 {
		return RuleEvaluation{}, fmt.Errorf("rule_evaluation: activated rule must have score_contribution > 0")
	}
	return RuleEvaluation{
		ruleID:            ruleID,
		ruleName:          ruleName,
		activated:         activated,
		scoreContribution: scoreContribution,
		reason:            reason,
	}, nil
}

func (r RuleEvaluation) RuleID() string         { return r.ruleID }
func (r RuleEvaluation) RuleName() string       { return r.ruleName }
func (r RuleEvaluation) Activated() bool        { return r.activated }
func (r RuleEvaluation) ScoreContribution() int { return r.scoreContribution }
func (r RuleEvaluation) Reason() string         { return r.reason }
