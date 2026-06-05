// Package valueobject contiene los Value Objects del BC Fraud Detection.
package valueobject

import "fmt"

// FraudDecision representa la decisión final del motor de reglas.
type FraudDecision string

const (
	DecisionApprove FraudDecision = "APPROVE" // Score < 50 — continuar con el adquirente
	DecisionReview  FraudDecision = "REVIEW"  // Score 50–69 — pasa pero queda marcado
	DecisionReject  FraudDecision = "REJECT"  // Score >= 70 — rechazar sin ir al adquirente
)

func ParseFraudDecision(s string) (FraudDecision, error) {
	switch FraudDecision(s) {
	case DecisionApprove, DecisionReview, DecisionReject:
		return FraudDecision(s), nil
	}
	return "", fmt.Errorf("unknown fraud decision %q", s)
}

func (d FraudDecision) String() string { return string(d) }

// ShouldReject indica si la transacción debe ser rechazada sin ir al adquirente.
func (d FraudDecision) ShouldReject() bool { return d == DecisionReject }

// ─── FraudScore ───────────────────────────────────────────────────────────────

// FraudScore encapsula el score calculado y la decisión resultante.
// Es inmutable una vez calculado.
type FraudScore struct {
	score    int // 0–100: a mayor score, mayor riesgo
	decision FraudDecision
	rulesHit []string // IDs de reglas que activaron (para auditoría)
}

// NewFraudScore crea un FraudScore calculando la decisión a partir del score.
// Umbrales:
//   - score < 50  → APPROVE
//   - score 50–69 → REVIEW
//   - score >= 70 → REJECT
func NewFraudScore(score int, rulesHit []string) (FraudScore, error) {
	if score < 0 || score > 100 {
		return FraudScore{}, fmt.Errorf("fraud_score: score %d out of range [0, 100]", score)
	}

	var decision FraudDecision
	switch {
	case score >= 70:
		decision = DecisionReject
	case score >= 50:
		decision = DecisionReview
	default:
		decision = DecisionApprove
	}

	return FraudScore{score: score, decision: decision, rulesHit: rulesHit}, nil
}

func (f FraudScore) Score() int              { return f.score }
func (f FraudScore) Decision() FraudDecision { return f.decision }
func (f FraudScore) RulesHit() []string      { return f.rulesHit }
func (f FraudScore) ShouldReject() bool      { return f.decision.ShouldReject() }
func (f FraudScore) IsZero() bool            { return f.decision == "" }
