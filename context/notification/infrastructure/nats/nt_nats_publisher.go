// Package nats contiene los adaptadores NATS del BC Notification.
package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

// EventPublisher implementa domain/service.EventPublisher usando NATS JetStream.
type EventPublisher struct {
	pub *natsutil.Publisher
}

func NewEventPublisher(pub *natsutil.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

// PublishDispatched publica el evento de auditoría NotificationDispatched.
// Es el único evento que publica el BC Notification — para trazabilidad.
func (p *EventPublisher) PublishDispatched(ctx context.Context, n *aggregate.Notification) error {
	payload := events.NotificationDispatchedPayload{
		NotificationID: n.ID(),
		TransactionID:  n.TransactionID().String(),
		Channel:        n.Channel().String(),
		Attempts:       n.AttemptCount(),
		DispatchedAt:   time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectNotificationDispatched,
		events.SubjectNotificationDispatched,
		n.ID(), "Notification",
		n.TransactionID().String(), "",
		payload,
	)
	if err != nil {
		return fmt.Errorf("nt publisher: publish NotificationDispatched: %w", err)
	}
	return nil
}
