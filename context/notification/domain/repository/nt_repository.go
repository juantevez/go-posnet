// Package repository define los puertos de salida del BC Notification.
package repository

import (
	"context"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// NotificationRepository persiste y recupera Notifications.
type NotificationRepository interface {
	// Save persiste una Notification nueva o actualiza una existente.
	Save(ctx context.Context, n *aggregate.Notification) error

	// FindByID recupera una Notification por su ID.
	FindByID(ctx context.Context, id string) (*aggregate.Notification, error)

	// FindByTransactionID recupera todas las notificaciones de una transacción.
	// Una transacción puede tener múltiples notificaciones (terminal + webhook + email).
	FindByTransactionID(ctx context.Context, txID domain.TransactionID) ([]*aggregate.Notification, error)

	// FindPendingRetries recupera notificaciones en estado RETRYING cuyo
	// NextRetryAt ya pasó. Usado por el job de reintento periódico.
	FindPendingRetries(ctx context.Context, limit int) ([]*aggregate.Notification, error)

	// FindDead recupera notificaciones en estado DEAD para revisión manual.
	FindDead(ctx context.Context, limit int) ([]*aggregate.Notification, error)
}
