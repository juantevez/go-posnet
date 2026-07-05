package command

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
	"github.com/juantevez/go-posnet/context/settlement/domain/service"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// AdminHandler implementa port.AdminService.
type AdminHandler struct {
	batchRepo repository.SettlementBatchRepository
	processor service.SettlementProcessor
}

func NewAdminHandler(batchRepo repository.SettlementBatchRepository, processor service.SettlementProcessor) *AdminHandler {
	return &AdminHandler{batchRepo: batchRepo, processor: processor}
}

// ForceClose fuerza el cierre de un batch desde operaciones de soporte.
func (h *AdminHandler) ForceClose(ctx context.Context, cmd port.ForceCloseCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.ForceClose")
	defer span.End()

	if cmd.BatchID == "" {
		return pkgerrors.NewValidationError("batch_id cannot be empty")
	}
	if cmd.OperatorID == "" {
		return pkgerrors.NewValidationError("operator_id cannot be empty for audit")
	}

	batch, err := h.batchRepo.FindByID(ctx, cmd.BatchID)
	if err != nil {
		return fmt.Errorf("ForceClose: find batch: %w", err)
	}
	if batch == nil {
		return pkgerrors.NewNotFoundError("SettlementBatch", cmd.BatchID)
	}

	if err := batch.RequestClose(); err != nil {
		return fmt.Errorf("ForceClose: request close: %w", err)
	}
	// Force close con totales del backend (0 del terminal — operación manual)
	if err := batch.Close(0, 0); err != nil {
		return fmt.Errorf("ForceClose: close: %w", err)
	}

	return h.batchRepo.Save(ctx, batch)
}

// ResubmitBatch reintenta el envío al procesador externo de un batch que quedó
// atascado en CLOSED — típicamente porque el envío original falló por una
// falla transitoria del procesador (ver docs/runbooks/batch-close-failure.md).
// No vuelve a calcular discrepancias: asume que el batch ya fue conciliado y
// solo faltó completar el envío.
func (h *AdminHandler) ResubmitBatch(ctx context.Context, cmd port.ResubmitBatchCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.ResubmitBatch")
	defer span.End()

	if cmd.BatchID == "" {
		return pkgerrors.NewValidationError("batch_id cannot be empty")
	}
	if cmd.OperatorID == "" {
		return pkgerrors.NewValidationError("operator_id cannot be empty for audit")
	}

	batch, err := h.batchRepo.FindByID(ctx, cmd.BatchID)
	if err != nil {
		return fmt.Errorf("ResubmitBatch: find batch: %w", err)
	}
	if batch == nil {
		return pkgerrors.NewNotFoundError("SettlementBatch", cmd.BatchID)
	}
	if batch.State() != valueobject.BatchStateClosed {
		return pkgerrors.NewValidationError(
			fmt.Sprintf("batch must be CLOSED to resubmit, current state is %s", batch.State()),
		)
	}

	confirmationID, err := h.processor.Submit(ctx, batch)
	if err != nil {
		return fmt.Errorf("ResubmitBatch: processor submit: %w", err)
	}
	if err := batch.Submit(); err != nil {
		return fmt.Errorf("ResubmitBatch: %w", err)
	}
	if err := h.batchRepo.Save(ctx, batch); err != nil {
		return fmt.Errorf("ResubmitBatch: save: %w", err)
	}

	observability.FromContext(ctx).Info("batch resubmitted to processor",
		slog.String("batch_id", batch.ID()),
		slog.String("operator_id", cmd.OperatorID),
		slog.String("confirmation_id", confirmationID),
	)
	return nil
}
