package aggregate

import (
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/entity"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ReconstituteParams contiene todos los campos para reconstruir
// una Notification desde la capa de persistencia.
type ReconstituteParams struct {
	ID            string
	TransactionID domain.TransactionID
	MerchantID    domain.MerchantID
	Channel       valueobject.NotificationChannel
	State         valueobject.NotificationState
	Receipt       valueobject.ReceiptPayload
	Attempts      []*entity.DeliveryAttempt
	MaxAttempts   int
	CreatedAt     time.Time
	DispatchedAt  *time.Time
	NextRetryAt   *time.Time
}

// Reconstitute reconstruye una Notification desde Postgres.
func Reconstitute(p ReconstituteParams) *Notification {
	return &Notification{
		id:            p.ID,
		transactionID: p.TransactionID,
		merchantID:    p.MerchantID,
		channel:       p.Channel,
		state:         p.State,
		receipt:       p.Receipt,
		attempts:      p.Attempts,
		maxAttempts:   p.MaxAttempts,
		createdAt:     p.CreatedAt,
		dispatchedAt:  p.DispatchedAt,
		nextRetryAt:   p.NextRetryAt,
	}
}
