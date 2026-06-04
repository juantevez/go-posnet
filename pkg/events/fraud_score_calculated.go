package events

// FraudScoreCalculatedPayload es el payload del evento posnet.fraud.score-calculated.v1
//
// Publicado por: Fraud Detection
// Consumido por: Authorization
//
// Contiene el score de riesgo calculado y la decisión del motor de reglas.
// Authorization usa Decision para continuar o cortar la Saga:
//   - APPROVE / REVIEW → continúa hacia el adquirente
//   - REJECT           → publica AuthorizationRejected directamente
type FraudScoreCalculatedPayload struct {
	TransactionID string   `json:"transaction_id"`
	Score         int      `json:"score"`        // 0–100: a mayor score, mayor riesgo
	Decision      string   `json:"decision"`     // APPROVE | REJECT | REVIEW
	RulesHit      []string `json:"rules_hit"`    // IDs de reglas que activaron (para auditoría)
	EvaluatedAt   string   `json:"evaluated_at"` // RFC3339 UTC
}
