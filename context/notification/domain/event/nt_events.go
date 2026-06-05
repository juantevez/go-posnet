// Package event contiene los Domain Events internos del BC Notification.
package event

import (
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// DomainEvent es la interfaz base.
type DomainEvent interface {
	EventType() string
	OccurredAt() time.Time
}

// ─── NotificationDispatched ───────────────────────────────────────────────────
// Emitido cuando la notificación fue entregada exitosamente.
// El adaptador NATS lo transforma en NotificationDispatchedPayload.

type NotificationDispatched struct {
	NotificationID string
	TransactionID  domain.TransactionID
	Channel        valueobject.NotificationChannel
	Attempts       int
	occurredAt     time.Time
}

func NewNotificationDispatched(
	notificationID string,
	transactionID domain.TransactionID,
	channel valueobject.NotificationChannel,
	attempts int,
) NotificationDispatched {
	return NotificationDispatched{
		NotificationID: notificationID,
		TransactionID:  transactionID,
		Channel:        channel,
		Attempts:       attempts,
		occurredAt:     time.Now().UTC(),
	}
}

func (e NotificationDispatched) EventType() string     { return "notification.dispatched" }
func (e NotificationDispatched) OccurredAt() time.Time { return e.occurredAt }

// ─── NotificationDead ─────────────────────────────────────────────────────────
// Emitido cuando la notificación superó el límite de reintentos.
// Genera una alerta operativa para revisión manual.

type NotificationDead struct {
	NotificationID string
	TransactionID  domain.TransactionID
	Channel        valueobject.NotificationChannel
	TotalAttempts  int
	occurredAt     time.Time
}

func NewNotificationDead(
	notificationID string,
	transactionID domain.TransactionID,
	channel valueobject.NotificationChannel,
	totalAttempts int,
) NotificationDead {
	return NotificationDead{
		NotificationID: notificationID,
		TransactionID:  transactionID,
		Channel:        channel,
		TotalAttempts:  totalAttempts,
		occurredAt:     time.Now().UTC(),
	}
}

func (e NotificationDead) EventType() string     { return "notification.dead" }
func (e NotificationDead) OccurredAt() time.Time { return e.occurredAt }
