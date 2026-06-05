// Package query contiene los query handlers del BC Notification.
package query

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/notification/application/port"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/repository"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// NotificationQueryHandler implementa las consultas de solo lectura del BC.
type NotificationQueryHandler struct {
	notifRepo repository.NotificationRepository
}

func NewNotificationQueryHandler(notifRepo repository.NotificationRepository) *NotificationQueryHandler {
	return &NotificationQueryHandler{notifRepo: notifRepo}
}

// GetNotification retorna el estado de una notificación por ID.
func (h *NotificationQueryHandler) GetNotification(
	ctx context.Context,
	id string,
) (*port.NotificationResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.GetNotification")
	defer span.End()

	if id == "" {
		return nil, pkgerrors.NewValidationError("notification_id cannot be empty")
	}

	n, err := h.notifRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("GetNotification: %w", err)
	}
	if n == nil {
		return nil, pkgerrors.NewNotFoundError("Notification", id)
	}

	return toResult(n), nil
}

// GetByTransactionID retorna todas las notificaciones de una transacción.
func (h *NotificationQueryHandler) GetByTransactionID(
	ctx context.Context,
	transactionID string,
) ([]*port.NotificationResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.GetByTransactionID")
	defer span.End()

	txID, err := domain.ParseTransactionID(transactionID)
	if err != nil {
		return nil, pkgerrors.NewValidationError("invalid transaction_id")
	}

	notifications, err := h.notifRepo.FindByTransactionID(ctx, txID)
	if err != nil {
		return nil, fmt.Errorf("GetByTransactionID: %w", err)
	}

	results := make([]*port.NotificationResult, 0, len(notifications))
	for _, n := range notifications {
		results = append(results, toResult(n))
	}
	return results, nil
}

// ListDead retorna notificaciones en estado DEAD para revisión manual.
func (h *NotificationQueryHandler) ListDead(
	ctx context.Context,
	limit int,
) ([]*port.NotificationResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.ListDead")
	defer span.End()

	if limit <= 0 {
		limit = 50
	}

	notifications, err := h.notifRepo.FindDead(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("ListDead: %w", err)
	}

	results := make([]*port.NotificationResult, 0, len(notifications))
	for _, n := range notifications {
		results = append(results, toResult(n))
	}
	return results, nil
}

// ─── Mapper ───────────────────────────────────────────────────────────────────

func toResult(n *aggregate.Notification) *port.NotificationResult {
	r := &port.NotificationResult{
		ID:            n.ID(),
		TransactionID: n.TransactionID().String(),
		MerchantID:    n.MerchantID().String(),
		Channel:       n.Channel().String(),
		State:         n.State().String(),
		AttemptCount:  n.AttemptCount(),
		MaxAttempts:   n.MaxAttempts(),
		CreatedAt:     n.CreatedAt().Format(time.RFC3339),
	}
	if t := n.DispatchedAt(); t != nil {
		r.DispatchedAt = t.Format(time.RFC3339)
	}
	if t := n.NextRetryAt(); t != nil {
		r.NextRetryAt = t.Format(time.RFC3339)
	}
	return r
}
