package command

import (
	"context"
	"fmt"

	"github.com/juantevez/go-posnet/context/notification/domain/repository"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// AdminHandler implementa las operaciones manuales del BC Notification.
type AdminHandler struct {
	notifRepo    repository.NotificationRepository
	notifHandler *NotifyHandler
}

func NewAdminHandler(
	notifRepo repository.NotificationRepository,
	notifHandler *NotifyHandler,
) *AdminHandler {
	return &AdminHandler{
		notifRepo:    notifRepo,
		notifHandler: notifHandler,
	}
}

// ForceRetry fuerza el reintento manual de una notificación DEAD.
// Solo disponible para operadores con permisos de administración.
func (h *AdminHandler) ForceRetry(ctx context.Context, notificationID string) error {
	ctx, span := observability.StartSpan(ctx, "command.ForceRetry")
	defer span.End()

	if notificationID == "" {
		return pkgerrors.NewValidationError("notification_id cannot be empty")
	}

	notif, err := h.notifRepo.FindByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("ForceRetry: find notification: %w", err)
	}
	if notif == nil {
		return pkgerrors.NewNotFoundError("Notification", notificationID)
	}

	// Re-despachar directamente — el dispatch actualiza el estado
	h.notifHandler.dispatch(ctx, notif)
	return nil
}
