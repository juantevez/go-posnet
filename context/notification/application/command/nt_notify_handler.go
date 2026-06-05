// Package command contiene los command handlers del BC Notification.
package command

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/notification/application/port"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/repository"
	"github.com/juantevez/go-posnet/context/notification/domain/service"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// NotifyHandler implementa port.NotificationService.
// Crea notificaciones, las despacha por el canal correspondiente
// y gestiona los reintentos con backoff exponencial.
type NotifyHandler struct {
	notifRepo   repository.NotificationRepository
	terminal    service.TerminalNotifier
	webhook     service.WebhookDispatcher
	publisher   service.EventPublisher
	idempotency *natsutil.IdempotencyStore
	pool        *pgxpool.Pool
}

func NewNotifyHandler(
	notifRepo repository.NotificationRepository,
	terminal service.TerminalNotifier,
	webhook service.WebhookDispatcher,
	publisher service.EventPublisher,
	idempotency *natsutil.IdempotencyStore,
	pool *pgxpool.Pool,
) *NotifyHandler {
	return &NotifyHandler{
		notifRepo:   notifRepo,
		terminal:    terminal,
		webhook:     webhook,
		publisher:   publisher,
		idempotency: idempotency,
		pool:        pool,
	}
}

// NotifyApproval crea y despacha las notificaciones de aprobación.
// Crea dos notificaciones: una al terminal (WebSocket) y otra al comercio (Webhook).
func (h *NotifyHandler) NotifyApproval(ctx context.Context, cmd port.NotifyApprovalCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.NotifyApproval")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("transaction_id", cmd.TransactionID),
		slog.String("event_id", cmd.EventID),
	)

	// Idempotencia
	already, err := h.idempotency.IsAlreadyProcessed(ctx, cmd.EventID)
	if err != nil {
		return fmt.Errorf("NotifyApproval: check idempotency: %w", err)
	}
	if already {
		log.Info("notification event already processed — skipping")
		return nil
	}

	txID, err := domain.ParseTransactionID(cmd.TransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid transaction_id")
	}
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id")
	}

	// Construir el ReceiptPayload
	receipt, err := valueobject.NewReceiptPayload(
		cmd.TransactionID, "", cmd.TerminalID,
		"APPROVED", cmd.AmountCents, cmd.Currency,
		cmd.CardLast4, cmd.CardNetwork, cmd.EntryMode, cmd.AuthorizedAt,
	)
	if err != nil {
		return pkgerrors.NewValidationError("invalid receipt payload: " + err.Error())
	}
	receipt.AuthCode = cmd.AuthCode

	// Crear notificación para el terminal (WebSocket)
	terminalNotif, err := aggregate.NewNotification(txID, merchantID, valueobject.ChannelTerminalWebSocket, receipt)
	if err != nil {
		return fmt.Errorf("NotifyApproval: create terminal notification: %w", err)
	}

	// Crear notificación para el comercio (Webhook)
	webhookNotif, err := aggregate.NewNotification(txID, merchantID, valueobject.ChannelWebhook, receipt)
	if err != nil {
		return fmt.Errorf("NotifyApproval: create webhook notification: %w", err)
	}

	// Persistir ambas + idempotencia (atómico)
	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		if err := h.notifRepo.Save(ctx, terminalNotif); err != nil {
			return err
		}
		if err := h.notifRepo.Save(ctx, webhookNotif); err != nil {
			return err
		}
		return h.idempotency.MarkAsProcessed(ctx, tx, cmd.EventID)
	})
	if err != nil {
		return fmt.Errorf("NotifyApproval: persist: %w", err)
	}

	// Despachar al terminal (best-effort — no bloquea si falla)
	go h.dispatch(context.Background(), terminalNotif)

	// Despachar webhook (best-effort)
	go h.dispatch(context.Background(), webhookNotif)

	log.Info("notifications created and dispatching",
		slog.String("terminal_notif_id", terminalNotif.ID()),
		slog.String("webhook_notif_id", webhookNotif.ID()),
	)
	return nil
}

// NotifyRejection crea y despacha las notificaciones de rechazo.
func (h *NotifyHandler) NotifyRejection(ctx context.Context, cmd port.NotifyRejectionCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.NotifyRejection")
	defer span.End()

	already, err := h.idempotency.IsAlreadyProcessed(ctx, cmd.EventID)
	if err != nil {
		return fmt.Errorf("NotifyRejection: check idempotency: %w", err)
	}
	if already {
		return nil
	}

	txID, err := domain.ParseTransactionID(cmd.TransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid transaction_id")
	}
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id")
	}

	receipt, err := valueobject.NewReceiptPayload(
		cmd.TransactionID, "", cmd.TerminalID,
		"REJECTED", cmd.AmountCents, cmd.Currency,
		cmd.CardLast4, cmd.CardNetwork, cmd.EntryMode, cmd.RejectedAt,
	)
	if err != nil {
		return pkgerrors.NewValidationError("invalid receipt payload: " + err.Error())
	}
	receipt.RejectionCode = cmd.RejectionCode
	receipt.RejectionReason = cmd.RejectionReason

	// Solo notificar al terminal — los rechazos no generan webhook al comercio
	terminalNotif, err := aggregate.NewNotification(txID, merchantID, valueobject.ChannelTerminalWebSocket, receipt)
	if err != nil {
		return fmt.Errorf("NotifyRejection: create notification: %w", err)
	}

	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		if err := h.notifRepo.Save(ctx, terminalNotif); err != nil {
			return err
		}
		return h.idempotency.MarkAsProcessed(ctx, tx, cmd.EventID)
	})
	if err != nil {
		return fmt.Errorf("NotifyRejection: persist: %w", err)
	}

	go h.dispatch(context.Background(), terminalNotif)
	return nil
}

// NotifyBatchClosed crea una notificación de cierre de lote para el comercio.
func (h *NotifyHandler) NotifyBatchClosed(ctx context.Context, cmd port.NotifyBatchClosedCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.NotifyBatchClosed")
	defer span.End()

	already, err := h.idempotency.IsAlreadyProcessed(ctx, cmd.EventID)
	if err != nil {
		return fmt.Errorf("NotifyBatchClosed: check idempotency: %w", err)
	}
	if already {
		return nil
	}

	// Para cierres de lote solo notificamos al comercio vía Webhook
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id")
	}

	// Usar un TransactionID sintético para el batch (basado en BatchID)
	batchTxID, _ := domain.ParseTransactionID(cmd.BatchID)

	// El receipt para batch close tiene un formato especial
	receipt := valueobject.ReceiptPayload{
		TransactionID: cmd.BatchID,
		Result:        "BATCH_CLOSED",
		AmountCents:   cmd.TotalAmount,
		Currency:      cmd.Currency,
		TransactionAt: cmd.ClosedAt,
	}

	webhookNotif, err := aggregate.NewNotification(batchTxID, merchantID, valueobject.ChannelWebhook, receipt)
	if err != nil {
		return fmt.Errorf("NotifyBatchClosed: create notification: %w", err)
	}

	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		if err := h.notifRepo.Save(ctx, webhookNotif); err != nil {
			return err
		}
		return h.idempotency.MarkAsProcessed(ctx, tx, cmd.EventID)
	})
	if err != nil {
		return fmt.Errorf("NotifyBatchClosed: persist: %w", err)
	}

	go h.dispatch(context.Background(), webhookNotif)
	return nil
}

// RetryFailed reintenta el envío de una notificación en estado RETRYING.
func (h *NotifyHandler) RetryFailed(ctx context.Context, notificationID string) error {
	ctx, span := observability.StartSpan(ctx, "command.RetryFailed")
	defer span.End()

	notif, err := h.notifRepo.FindByID(ctx, notificationID)
	if err != nil {
		return fmt.Errorf("RetryFailed: find notification: %w", err)
	}
	if notif == nil {
		return pkgerrors.NewNotFoundError("Notification", notificationID)
	}

	h.dispatch(ctx, notif)
	return nil
}

// ─── dispatch — lógica de entrega ────────────────────────────────────────────

// dispatch ejecuta el envío al canal correspondiente y actualiza el estado.
func (h *NotifyHandler) dispatch(ctx context.Context, n *aggregate.Notification) {
	log := observability.FromContext(ctx).With(
		slog.String("notification_id", n.ID()),
		slog.String("channel", n.Channel().String()),
		slog.String("transaction_id", n.TransactionID().String()),
	)

	var delivered bool
	var httpStatus int
	var dispatchErr error

	switch n.Channel() {
	case valueobject.ChannelTerminalWebSocket:
		var reason string
		delivered, reason, dispatchErr = h.terminal.SendReceipt(ctx, n)
		if !delivered && dispatchErr == nil {
			dispatchErr = fmt.Errorf("terminal not connected: %s", reason)
		}

	case valueobject.ChannelWebhook:
		httpStatus, dispatchErr = h.webhook.Dispatch(ctx, n)
		delivered = dispatchErr == nil && httpStatus >= 200 && httpStatus < 300
		if !delivered && dispatchErr == nil {
			dispatchErr = fmt.Errorf("webhook returned HTTP %d", httpStatus)
		}

	default:
		log.Warn("unknown channel — skipping dispatch")
		return
	}

	// Actualizar el aggregate según el resultado
	var updateErr error
	if delivered {
		updateErr = n.MarkSent(httpStatus)
		if updateErr == nil {
			log.Info("notification delivered successfully",
				slog.Int("attempt", n.AttemptCount()),
			)
			// Publicar evento de auditoría NotificationDispatched a NATS
			if err := h.publisher.PublishDispatched(ctx, n); err != nil {
				log.Error("failed to publish NotificationDispatched",
					slog.String("error", err.Error()),
				)
			}
		}
	} else {
		errMsg := ""
		if dispatchErr != nil {
			errMsg = dispatchErr.Error()
		}
		updateErr = n.MarkFailed(httpStatus, errMsg)
		if updateErr == nil {
			log.Warn("notification delivery failed",
				slog.Int("attempt", n.AttemptCount()),
				slog.String("state", n.State().String()),
				slog.String("error", errMsg),
			)
		}
	}

	if updateErr != nil {
		observability.RecordError(ctx, updateErr)
		log.Error("failed to update notification state", slog.String("error", updateErr.Error()))
		return
	}

	// Persistir el estado actualizado
	if err := h.notifRepo.Save(ctx, n); err != nil {
		observability.RecordError(ctx, err)
		log.Error("failed to save notification", slog.String("error", err.Error()))
	}
}
