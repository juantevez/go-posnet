// Package query contiene los query handlers del BC Settlement.
package query

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// BatchQueryHandler implementa las consultas de solo lectura del BC Settlement.
type BatchQueryHandler struct {
	batchRepo repository.SettlementBatchRepository
}

func NewBatchQueryHandler(batchRepo repository.SettlementBatchRepository) *BatchQueryHandler {
	return &BatchQueryHandler{batchRepo: batchRepo}
}

// GetBatch retorna el estado de un batch por su ID.
func (h *BatchQueryHandler) GetBatch(ctx context.Context, batchID string) (*port.BatchResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.GetBatch")
	defer span.End()

	if batchID == "" {
		return nil, pkgerrors.NewValidationError("batch_id cannot be empty")
	}

	batch, err := h.batchRepo.FindByID(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("GetBatch: %w", err)
	}
	if batch == nil {
		return nil, pkgerrors.NewNotFoundError("SettlementBatch", batchID)
	}

	result := &port.BatchResult{
		ID:            batch.ID(),
		TerminalID:    batch.TerminalID().String(),
		MerchantID:    batch.MerchantID().String(),
		BatchDate:     batch.BatchDate().Format("2006-01-02"),
		State:         batch.State().String(),
		Currency:      batch.Currency(),
		Discrepancies: batch.Discrepancies(),
	}

	if s := batch.Summary(); s != nil {
		result.TotalCount = s.TotalCount()
		result.TotalAmount = s.TotalAmount().Cents()
	}

	if t := batch.ClosedAt(); t != nil {
		result.ClosedAt = t.Format(time.RFC3339)
	}
	if t := batch.SubmittedAt(); t != nil {
		result.SubmittedAt = t.Format(time.RFC3339)
	}
	if t := batch.SettledAt(); t != nil {
		result.SettledAt = t.Format(time.RFC3339)
	}

	return result, nil
}

// ListBatchesByMerchant lista los batches de un comercio en una fecha.
func (h *BatchQueryHandler) ListBatchesByMerchant(
	ctx context.Context,
	cmd port.ListBatchesCommand,
) ([]*port.BatchResult, error) {
	ctx, span := observability.StartSpan(ctx, "query.ListBatchesByMerchant")
	defer span.End()

	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return nil, pkgerrors.NewValidationError("invalid merchant_id")
	}

	date, err := time.Parse("2006-01-02", cmd.Date)
	if err != nil {
		return nil, pkgerrors.NewValidationError("invalid date format — use YYYY-MM-DD")
	}

	batches, err := h.batchRepo.ListByMerchantDate(ctx, merchantID, date)
	if err != nil {
		return nil, fmt.Errorf("ListBatchesByMerchant: %w", err)
	}

	results := make([]*port.BatchResult, 0, len(batches))
	for _, b := range batches {
		r := &port.BatchResult{
			ID:            b.ID(),
			TerminalID:    b.TerminalID().String(),
			MerchantID:    b.MerchantID().String(),
			BatchDate:     b.BatchDate().Format("2006-01-02"),
			State:         b.State().String(),
			Currency:      b.Currency(),
			Discrepancies: b.Discrepancies(),
		}
		if s := b.Summary(); s != nil {
			r.TotalCount = s.TotalCount()
			r.TotalAmount = s.TotalAmount().Cents()
		}
		if t := b.ClosedAt(); t != nil {
			r.ClosedAt = t.Format(time.RFC3339)
		}
		results = append(results, r)
	}

	return results, nil
}
