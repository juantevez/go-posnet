// Package command contiene los command handlers del BC Settlement.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/domain/aggregate"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
	"github.com/juantevez/go-posnet/context/settlement/domain/service"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// BatchHandler implementa port.BatchService.
// Orquesta el ciclo de vida de los batches de liquidación.
type BatchHandler struct {
	batchRepo   repository.SettlementBatchRepository
	publisher   service.EventPublisher
	processor   service.SettlementProcessor
	idempotency *natsutil.IdempotencyStore
	pool        pgutil.PgxPool
	mdrPercent  float64
	metrics     *Metrics // opcional; nil = sin instrumentación (tests)
}

// SetMetrics inyecta los instrumentos de negocio (desde el wiring, tras InitMeter).
func (h *BatchHandler) SetMetrics(m *Metrics) {
	h.metrics = m
}

func NewBatchHandler(
	batchRepo repository.SettlementBatchRepository,
	publisher service.EventPublisher,
	processor service.SettlementProcessor,
	idempotency *natsutil.IdempotencyStore,
	pool pgutil.PgxPool,
	mdrPercent float64,
) *BatchHandler {
	return &BatchHandler{
		batchRepo:   batchRepo,
		publisher:   publisher,
		processor:   processor,
		idempotency: idempotency,
		pool:        pool,
		mdrPercent:  mdrPercent,
	}
}

// RegisterApproval agrega una transacción aprobada al batch del terminal.
func (h *BatchHandler) RegisterApproval(ctx context.Context, cmd port.RegisterApprovalCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.RegisterApproval")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("transaction_id", cmd.TransactionID),
		slog.String("terminal_id", cmd.TerminalID),
		slog.String("event_id", cmd.EventID),
	)

	// Parsear Value Objects
	terminalID, err := domain.ParseTerminalID(cmd.TerminalID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid terminal_id")
	}
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id")
	}
	txID, err := domain.ParseTransactionID(cmd.TransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid transaction_id")
	}
	authorizedAt, err := time.Parse(time.RFC3339, cmd.AuthorizedAt)
	if err != nil {
		authorizedAt = time.Now().UTC()
	}

	var batchID string
	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, tx, cmd.EventID)
		if err != nil {
			return err
		}
		if !inserted {
			log.Info("approval event already registered — skipping")
			return nil
		}

		batch, err := h.batchRepo.FindOrCreateOpen(ctx, terminalID, merchantID, authorizedAt, cmd.Currency)
		if err != nil {
			return fmt.Errorf("find or create batch: %w", err)
		}
		if err := batch.AddTransaction(txID, cmd.AmountCents, valueobject.BatchTxPurchase); err != nil {
			return fmt.Errorf("add transaction: %w", err)
		}
		batchID = batch.ID()
		return h.batchRepo.Save(ctx, batch)
	})
	if err != nil {
		observability.RecordError(ctx, err)
		return fmt.Errorf("RegisterApproval: %w", err)
	}

	if batchID != "" {
		h.metrics.RecordAuthApproved(ctx)
		log.Info("transaction registered in batch",
			slog.String("batch_id", batchID),
			slog.Int64("amount_cents", cmd.AmountCents),
		)
	}
	return nil
}

// RegisterReversal descuenta una anulación del batch del terminal.
func (h *BatchHandler) RegisterReversal(ctx context.Context, cmd port.RegisterReversalCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.RegisterReversal")
	defer span.End()

	terminalID, err := domain.ParseTerminalID(cmd.TerminalID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid terminal_id")
	}
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id")
	}
	origTxID, err := domain.ParseTransactionID(cmd.OriginalTransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid original_transaction_id")
	}
	completedAt, err := time.Parse(time.RFC3339, cmd.CompletedAt)
	if err != nil {
		completedAt = time.Now().UTC()
	}

	var registered bool
	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, tx, cmd.EventID)
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}

		batch, err := h.batchRepo.FindOrCreateOpen(ctx, terminalID, merchantID, completedAt, cmd.Currency)
		if err != nil {
			return fmt.Errorf("find or create batch: %w", err)
		}
		if err := batch.RemoveTransaction(origTxID); err != nil {
			return fmt.Errorf("remove transaction: %w", err)
		}
		registered = true
		return h.batchRepo.Save(ctx, batch)
	})
	if err != nil {
		return err
	}
	if registered {
		h.metrics.RecordReversal(ctx)
	}
	return nil
}

// ProcessBatchClose procesa el cierre de lote solicitado por el terminal.
func (h *BatchHandler) ProcessBatchClose(ctx context.Context, cmd port.ProcessBatchCloseCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.ProcessBatchClose")
	defer span.End()

	closeStart := time.Now()

	log := observability.FromContext(ctx).With(
		slog.String("terminal_id", cmd.TerminalID),
		slog.String("batch_date", cmd.BatchDate),
		slog.String("event_id", cmd.EventID),
	)

	terminalID, err := domain.ParseTerminalID(cmd.TerminalID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid terminal_id")
	}

	batchDate, err := time.Parse("2006-01-02", cmd.BatchDate)
	if err != nil {
		batchDate = time.Now().UTC()
	}

	// Recuperar el batch OPEN del terminal
	batch, err := h.batchRepo.FindOpenByTerminal(ctx, terminalID, batchDate)
	if err != nil {
		return fmt.Errorf("ProcessBatchClose: find open batch: %w", err)
	}
	if batch == nil {
		log.Warn("no open batch found for terminal — nothing to close")
		return nil
	}

	// Solicitar cierre → calcular totales → comparar con terminal
	if err := batch.RequestClose(); err != nil {
		return fmt.Errorf("ProcessBatchClose: request close: %w", err)
	}
	if err := batch.Close(cmd.TerminalCount, cmd.TerminalAmount); err != nil {
		return fmt.Errorf("ProcessBatchClose: close batch: %w", err)
	}

	processed := false
	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, tx, cmd.EventID)
		if err != nil {
			return err
		}
		if !inserted {
			log.Info("batch close event already processed — skipping")
			return nil
		}
		processed = true
		return h.batchRepo.Save(ctx, batch)
	})
	if err != nil {
		return fmt.Errorf("ProcessBatchClose: %w", err)
	}
	if !processed {
		return nil
	}
	h.metrics.RecordBatchClosed(ctx, batch.Currency(), batch.State().String(), time.Since(closeStart))

	// Publicar BatchClosed a NATS → Notification BC
	if err := h.publisher.PublishBatchClosed(ctx, batch); err != nil {
		log.Error("failed to publish BatchClosed", slog.String("error", err.Error()))
	}

	log.Info("batch closed successfully",
		slog.String("batch_id", batch.ID()),
		slog.Int("discrepancies", batch.Discrepancies()),
	)

	// Si hay discrepancias, marcar como DISPUTED en lugar de continuar
	if batch.Discrepancies() > 0 {
		log.Warn("batch has discrepancies — marking as disputed",
			slog.Int("discrepancies", batch.Discrepancies()),
		)
		if err := batch.MarkDisputed("terminal count/amount mismatch"); err != nil {
			return fmt.Errorf("ProcessBatchClose: mark disputed: %w", err)
		}
		return h.batchRepo.Save(ctx, batch)
	}

	// Sin discrepancias → enviar al procesador externo
	return h.submitBatch(ctx, batch)
}

// submitBatch envía la remesa al procesador externo y, si acepta el envío,
// transiciona el batch a SUBMITTED y persiste el cambio.
func (h *BatchHandler) submitBatch(ctx context.Context, batch *aggregate.SettlementBatch) error {
	confirmationID, err := h.processor.Submit(ctx, batch)
	if err != nil {
		return fmt.Errorf("submitBatch: processor submit: %w", err)
	}

	if err := batch.Submit(); err != nil {
		return fmt.Errorf("submitBatch: %w", err)
	}
	if err := h.batchRepo.Save(ctx, batch); err != nil {
		return fmt.Errorf("submitBatch: save: %w", err)
	}

	observability.FromContext(ctx).Info("batch submitted to processor",
		slog.String("batch_id", batch.ID()),
		slog.String("confirmation_id", confirmationID),
	)
	return nil
}
