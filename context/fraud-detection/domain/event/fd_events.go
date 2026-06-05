// Package event contiene los Domain Events internos del BC Fraud Detection.
package event

import (
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// DomainEvent es la interfaz base de los eventos de dominio internos.
type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
}

// ─── FraudCaseEvaluated ───────────────────────────────────────────────────────
// Emitido cuando el motor de reglas termina de evaluar una transacción.
// El adaptador NATS lo transforma en FraudScoreCalculatedPayload y lo publica.

type FraudCaseEvaluated struct {
	FraudCaseID   string
	TransactionID domain.TransactionID
	Score         valueobject.FraudScore
	occurredAt    time.Time
}

func NewFraudCaseEvaluated(
	fraudCaseID string,
	transactionID domain.TransactionID,
	score valueobject.FraudScore,
) FraudCaseEvaluated {
	return FraudCaseEvaluated{
		FraudCaseID:   fraudCaseID,
		TransactionID: transactionID,
		Score:         score,
		occurredAt:    time.Now().UTC(),
	}
}

func (e FraudCaseEvaluated) EventType() string     { return "fraud_case.evaluated" }
func (e FraudCaseEvaluated) OccurredAt() time.Time { return e.occurredAt }
