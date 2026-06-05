package command

import (
	"context"
	"fmt"

	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// AdminHandler implementa port.AdminService.
type AdminHandler struct {
	batchRepo repository.SettlementBatchRepository
}

func NewAdminHandler(batchRepo repository.SettlementBatchRepository) *AdminHandler {
	return &AdminHandler{batchRepo: batchRepo}
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
