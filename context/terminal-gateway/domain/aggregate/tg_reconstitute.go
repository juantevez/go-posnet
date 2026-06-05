package aggregate

import (
	"time"

	"github.com/tu-org/posnet-backend/context/terminal-gateway/domain/valueobject"
	"github.com/tu-org/posnet-backend/pkg/domain"
)

// ReconstituteParams contiene todos los campos para reconstruir
// una PaymentSession desde la capa de persistencia.
type ReconstituteParams struct {
	ID              domain.TransactionID
	TerminalID      domain.TerminalID
	MerchantID      domain.MerchantID
	Amount          domain.Money
	STAN            domain.STAN
	Channel         valueobject.PaymentChannel
	State           valueobject.SessionState
	AuthCode        string
	RejectionCode   string
	RejectionReason string
	ExpiresAt       time.Time
	CreatedAt       time.Time
	ClosedAt        *time.Time
}

// Reconstitute reconstruye una PaymentSession desde Postgres.
// No emite eventos de dominio ni valida invariantes de creación.
func Reconstitute(p ReconstituteParams) *PaymentSession {
	return &PaymentSession{
		id:              p.ID,
		terminalID:      p.TerminalID,
		merchantID:      p.MerchantID,
		amount:          p.Amount,
		stan:            p.STAN,
		channel:         p.Channel,
		state:           p.State,
		authCode:        p.AuthCode,
		rejectionCode:   p.RejectionCode,
		rejectionReason: p.RejectionReason,
		expiresAt:       p.ExpiresAt,
		createdAt:       p.CreatedAt,
		closedAt:        p.ClosedAt,
	}
}
