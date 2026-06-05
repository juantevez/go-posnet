package aggregate

import (
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
)

// BuildNeutralScore aplica un score neutro al FraudCase cuando el motor
// de reglas falla completamente (error de infraestructura).
// Score 50 → decisión REVIEW: la transacción continúa pero queda marcada.
// reason documenta el motivo del bypass para auditoría.
func (fc *FraudCase) BuildNeutralScore(reason string) (valueobject.FraudScore, error) {
	neutralEval, err := valueobject.NewRuleEvaluation(
		"BYPASS", "Engine Bypass", true, 50,
		"rule engine failed: "+reason,
	)
	if err != nil {
		return valueobject.FraudScore{}, err
	}

	if err := fc.ApplyEvaluations([]valueobject.RuleEvaluation{neutralEval}); err != nil {
		return valueobject.FraudScore{}, err
	}

	return fc.score, nil
}
