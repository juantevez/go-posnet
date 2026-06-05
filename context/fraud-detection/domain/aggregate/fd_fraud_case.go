// Package aggregate contiene los Aggregates del BC Fraud Detection.
package aggregate

import (
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/event"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// FraudCase es el Aggregate Root del BC Fraud Detection.
// Representa el análisis de fraude de una transacción específica.
// Contiene el input de la transacción, cada RuleEvaluation ejecutada
// y el FraudScore final calculado.
type FraudCase struct {
	// Identidad
	id            string               // UUID propio del FraudCase
	transactionID domain.TransactionID // Correlación con la transacción

	// Input de la transacción
	terminalID  domain.TerminalID
	merchantID  domain.MerchantID
	amountCents int64
	currency    string
	cardNetwork string
	entryMode   string
	occurredAt  time.Time

	// Resultado del análisis
	evaluations []valueobject.RuleEvaluation
	score       valueobject.FraudScore

	// Estado
	evaluated   bool
	evaluatedAt *time.Time

	// Eventos pendientes
	domainEvents []event.DomainEvent
}

// NewFraudCase crea un FraudCase en estado pendiente de evaluación.
func NewFraudCase(
	transactionID domain.TransactionID,
	terminalID domain.TerminalID,
	merchantID domain.MerchantID,
	amountCents int64,
	currency string,
	cardNetwork string,
	entryMode string,
	occurredAt time.Time,
) (*FraudCase, error) {
	if transactionID.IsZero() {
		return nil, fmt.Errorf("fraud_case: transaction_id cannot be zero")
	}
	if amountCents <= 0 {
		return nil, fmt.Errorf("fraud_case: amount_cents must be positive")
	}

	return &FraudCase{
		id:            domain.NewTransactionID().String(), // UUID propio del caso
		transactionID: transactionID,
		terminalID:    terminalID,
		merchantID:    merchantID,
		amountCents:   amountCents,
		currency:      currency,
		cardNetwork:   cardNetwork,
		entryMode:     entryMode,
		occurredAt:    occurredAt,
		evaluated:     false,
	}, nil
}

// ApplyEvaluations registra los resultados de todas las reglas y calcula el score final.
// Solo puede llamarse una vez — el FraudCase es inmutable tras la evaluación.
func (fc *FraudCase) ApplyEvaluations(evaluations []valueobject.RuleEvaluation) error {
	if fc.evaluated {
		return fmt.Errorf("fraud_case %s: already evaluated", fc.id)
	}
	if len(evaluations) == 0 {
		return fmt.Errorf("fraud_case %s: evaluations cannot be empty", fc.id)
	}

	// Sumar contribuciones de reglas activadas
	totalScore := 0
	rulesHit := []string{}

	for _, eval := range evaluations {
		if eval.Activated() {
			totalScore += eval.ScoreContribution()
			rulesHit = append(rulesHit, eval.RuleID())
		}
	}

	// Clampear a 100
	if totalScore > 100 {
		totalScore = 100
	}

	score, err := valueobject.NewFraudScore(totalScore, rulesHit)
	if err != nil {
		return fmt.Errorf("fraud_case: calculate score: %w", err)
	}

	fc.evaluations = evaluations
	fc.score = score
	fc.evaluated = true
	now := time.Now().UTC()
	fc.evaluatedAt = &now

	fc.record(event.NewFraudCaseEvaluated(fc.id, fc.transactionID, score))
	return nil
}

func (fc *FraudCase) record(e event.DomainEvent) {
	fc.domainEvents = append(fc.domainEvents, e)
}

// ─── Getters ──────────────────────────────────────────────────────────────────

func (fc *FraudCase) ID() string                                { return fc.id }
func (fc *FraudCase) TransactionID() domain.TransactionID       { return fc.transactionID }
func (fc *FraudCase) TerminalID() domain.TerminalID             { return fc.terminalID }
func (fc *FraudCase) MerchantID() domain.MerchantID             { return fc.merchantID }
func (fc *FraudCase) AmountCents() int64                        { return fc.amountCents }
func (fc *FraudCase) Currency() string                          { return fc.currency }
func (fc *FraudCase) CardNetwork() string                       { return fc.cardNetwork }
func (fc *FraudCase) EntryMode() string                         { return fc.entryMode }
func (fc *FraudCase) OccurredAt() time.Time                     { return fc.occurredAt }
func (fc *FraudCase) Evaluations() []valueobject.RuleEvaluation { return fc.evaluations }
func (fc *FraudCase) Score() valueobject.FraudScore             { return fc.score }
func (fc *FraudCase) IsEvaluated() bool                         { return fc.evaluated }
func (fc *FraudCase) EvaluatedAt() *time.Time                   { return fc.evaluatedAt }
func (fc *FraudCase) DomainEvents() []event.DomainEvent         { return fc.domainEvents }
func (fc *FraudCase) ClearDomainEvents()                        { fc.domainEvents = nil }
