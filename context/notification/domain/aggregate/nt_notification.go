// Package aggregate contiene los Aggregates del BC Notification.
package aggregate

import (
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/entity"
	"github.com/juantevez/go-posnet/context/notification/domain/event"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// maxAttempts es el número máximo de intentos antes de marcar como DEAD.
const maxAttempts = 5

// Notification es el Aggregate Root del BC Notification.
// Representa una notificación a despachar por un canal específico.
// Gestiona la política de reintentos con backoff exponencial.
type Notification struct {
	id            string
	transactionID domain.TransactionID
	merchantID    domain.MerchantID
	channel       valueobject.NotificationChannel
	state         valueobject.NotificationState
	receipt       valueobject.ReceiptPayload
	attempts      []*entity.DeliveryAttempt
	maxAttempts   int
	createdAt     time.Time
	dispatchedAt  *time.Time
	nextRetryAt   *time.Time

	domainEvents []event.DomainEvent
}

// NewNotification crea una Notification en estado PENDING.
func NewNotification(
	transactionID domain.TransactionID,
	merchantID domain.MerchantID,
	channel valueobject.NotificationChannel,
	receipt valueobject.ReceiptPayload,
) (*Notification, error) {
	if transactionID.IsZero() {
		return nil, fmt.Errorf("notification: transaction_id cannot be zero")
	}

	n := &Notification{
		id:            domain.NewTransactionID().String(),
		transactionID: transactionID,
		merchantID:    merchantID,
		channel:       channel,
		state:         valueobject.StatePending,
		receipt:       receipt,
		maxAttempts:   maxAttempts,
		createdAt:     time.Now().UTC(),
	}
	return n, nil
}

// ─── Transiciones de estado ───────────────────────────────────────────────────

// MarkSent marca la notificación como entregada exitosamente.
func (n *Notification) MarkSent(httpStatus int) error {
	if err := n.transition(valueobject.StateSent); err != nil {
		return err
	}
	attempt, _ := entity.NewDeliveryAttempt(n.id, n.currentAttemptNumber(), true, httpStatus, "")
	n.attempts = append(n.attempts, attempt)

	now := time.Now().UTC()
	n.dispatchedAt = &now

	n.record(event.NewNotificationDispatched(n.id, n.transactionID, n.channel, len(n.attempts)))
	return nil
}

// MarkFailed registra un intento fallido y decide si reintentar o marcar como DEAD.
func (n *Notification) MarkFailed(httpStatus int, errorMessage string) error {
	attempt, _ := entity.NewDeliveryAttempt(n.id, n.currentAttemptNumber(), false, httpStatus, errorMessage)
	n.attempts = append(n.attempts, attempt)

	if len(n.attempts) >= n.maxAttempts {
		// Superó el límite — marcar como DEAD
		if err := n.transition(valueobject.StateDead); err != nil {
			return err
		}
		n.record(event.NewNotificationDead(n.id, n.transactionID, n.channel, len(n.attempts)))
		return nil
	}

	// Calcular próximo reintento con backoff exponencial
	backoff := n.calculateBackoff(len(n.attempts))
	nextRetry := time.Now().UTC().Add(backoff)
	n.nextRetryAt = &nextRetry

	if err := n.transition(valueobject.StateRetrying); err != nil {
		return err
	}
	return nil
}

// calculateBackoff retorna el tiempo de espera para el intento N.
// Política: 30s → 2m → 10m → 1h → DEAD
func (n *Notification) calculateBackoff(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 30 * time.Second
	case 2:
		return 2 * time.Minute
	case 3:
		return 10 * time.Minute
	case 4:
		return 1 * time.Hour
	default:
		return 1 * time.Hour
	}
}

// currentAttemptNumber retorna el número del intento actual (1-based).
func (n *Notification) currentAttemptNumber() int {
	return len(n.attempts) + 1
}

func (n *Notification) transition(next valueobject.NotificationState) error {
	if !n.state.CanTransitionTo(next) {
		return fmt.Errorf("notification %s: invalid transition %s → %s", n.id, n.state, next)
	}
	n.state = next
	return nil
}

func (n *Notification) record(e event.DomainEvent) {
	n.domainEvents = append(n.domainEvents, e)
}

// ─── Getters ──────────────────────────────────────────────────────────────────

func (n *Notification) ID() string                               { return n.id }
func (n *Notification) TransactionID() domain.TransactionID      { return n.transactionID }
func (n *Notification) MerchantID() domain.MerchantID            { return n.merchantID }
func (n *Notification) Channel() valueobject.NotificationChannel { return n.channel }
func (n *Notification) State() valueobject.NotificationState     { return n.state }
func (n *Notification) Receipt() valueobject.ReceiptPayload      { return n.receipt }
func (n *Notification) Attempts() []*entity.DeliveryAttempt      { return n.attempts }
func (n *Notification) AttemptCount() int                        { return len(n.attempts) }
func (n *Notification) MaxAttempts() int                         { return n.maxAttempts }
func (n *Notification) CreatedAt() time.Time                     { return n.createdAt }
func (n *Notification) DispatchedAt() *time.Time                 { return n.dispatchedAt }
func (n *Notification) NextRetryAt() *time.Time                  { return n.nextRetryAt }
func (n *Notification) DomainEvents() []event.DomainEvent        { return n.domainEvents }
func (n *Notification) ClearDomainEvents()                       { n.domainEvents = nil }
