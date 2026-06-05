// Package entity contiene las entidades del BC Notification.
package entity

import (
	"fmt"
	"time"
)

// DeliveryAttempt registra el resultado de un intento de entrega de una notificación.
// Pertenece al aggregate Notification — no es un Aggregate Root.
type DeliveryAttempt struct {
	id             string
	notificationID string
	attemptNumber  int
	success        bool
	httpStatus     int    // Solo para webhooks (0 si no aplica)
	errorMessage   string // Si success == false
	attemptedAt    time.Time
}

// NewDeliveryAttempt crea un intento de entrega.
func NewDeliveryAttempt(
	notificationID string,
	attemptNumber int,
	success bool,
	httpStatus int,
	errorMessage string,
) (*DeliveryAttempt, error) {
	if notificationID == "" {
		return nil, fmt.Errorf("delivery_attempt: notification_id cannot be empty")
	}
	if attemptNumber < 1 {
		return nil, fmt.Errorf("delivery_attempt: attempt_number must be >= 1")
	}
	return &DeliveryAttempt{
		id:             fmt.Sprintf("%s-%d", notificationID, attemptNumber),
		notificationID: notificationID,
		attemptNumber:  attemptNumber,
		success:        success,
		httpStatus:     httpStatus,
		errorMessage:   errorMessage,
		attemptedAt:    time.Now().UTC(),
	}, nil
}

// ReconstituteDeliveryAttempt reconstruye desde Postgres.
func ReconstituteDeliveryAttempt(
	id, notificationID string,
	attemptNumber int,
	success bool,
	httpStatus int,
	errorMessage string,
	attemptedAt time.Time,
) *DeliveryAttempt {
	return &DeliveryAttempt{
		id:             id,
		notificationID: notificationID,
		attemptNumber:  attemptNumber,
		success:        success,
		httpStatus:     httpStatus,
		errorMessage:   errorMessage,
		attemptedAt:    attemptedAt,
	}
}

func (d *DeliveryAttempt) ID() string             { return d.id }
func (d *DeliveryAttempt) NotificationID() string { return d.notificationID }
func (d *DeliveryAttempt) AttemptNumber() int     { return d.attemptNumber }
func (d *DeliveryAttempt) Success() bool          { return d.success }
func (d *DeliveryAttempt) HTTPStatus() int        { return d.httpStatus }
func (d *DeliveryAttempt) ErrorMessage() string   { return d.errorMessage }
func (d *DeliveryAttempt) AttemptedAt() time.Time { return d.attemptedAt }
