package aggregate

import (
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ReconstituteParams contiene todos los campos para reconstruir
// un FraudCase desde la capa de persistencia.
type ReconstituteParams struct {
	ID            string
	TransactionID domain.TransactionID
	TerminalID    domain.TerminalID
	MerchantID    domain.MerchantID
	AmountCents   int64
	Currency      string
	CardNetwork   string
	EntryMode     string
	OccurredAt    time.Time
	Score         valueobject.FraudScore
	Evaluations   []valueobject.RuleEvaluation
	EvaluatedAt   *time.Time
}

// Reconstitute reconstruye un FraudCase desde Postgres.
// No ejecuta el motor de reglas ni valida invariantes de creación.
func Reconstitute(p ReconstituteParams) *FraudCase {
	return &FraudCase{
		id:            p.ID,
		transactionID: p.TransactionID,
		terminalID:    p.TerminalID,
		merchantID:    p.MerchantID,
		amountCents:   p.AmountCents,
		currency:      p.Currency,
		cardNetwork:   p.CardNetwork,
		entryMode:     p.EntryMode,
		occurredAt:    p.OccurredAt,
		score:         p.Score,
		evaluations:   p.Evaluations,
		evaluated:     true,
		evaluatedAt:   p.EvaluatedAt,
	}
}
