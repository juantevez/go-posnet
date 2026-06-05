// Package nats contiene los adaptadores NATS del BC Settlement.
package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

// EventPublisher implementa domain/service.EventPublisher usando NATS JetStream.
type EventPublisher struct {
	pub *natsutil.Publisher
}

func NewEventPublisher(pub *natsutil.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

// PublishBatchClosed publica al stream POSNET_SETTLEMENT.
// Consumido por: Notification BC.
func (p *EventPublisher) PublishBatchClosed(ctx context.Context, b *aggregate.SettlementBatch) error {
	if b.Summary() == nil {
		return fmt.Errorf("st publisher: cannot publish BatchClosed — summary is nil")
	}

	payload := events.BatchClosedPayload{
		BatchID:       b.ID(),
		TerminalID:    b.TerminalID().String(),
		MerchantID:    b.MerchantID().String(),
		BatchDate:     b.BatchDate().Format("2006-01-02"),
		TotalCount:    b.Summary().TotalCount(),
		TotalAmount:   b.Summary().TotalAmount().Cents(),
		Currency:      b.Currency(),
		Discrepancies: b.Discrepancies(),
		ClosedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectBatchClosed,
		events.SubjectBatchClosed,
		b.ID(), "SettlementBatch",
		b.ID(), "",
		payload,
	)
	if err != nil {
		return fmt.Errorf("st publisher: publish BatchClosed: %w", err)
	}
	return nil
}

// PublishSettlementCompleted publica el resumen de liquidación diaria.
// Consumido por: Notification BC.
func (p *EventPublisher) PublishSettlementCompleted(
	ctx context.Context,
	merchantID, settlementDate string,
	totalBatches int,
	totalAmount, netAmount int64,
	currency string,
	mdrPercent float64,
) error {
	payload := events.SettlementCompletedPayload{
		MerchantID:     merchantID,
		SettlementDate: settlementDate,
		TotalBatches:   totalBatches,
		TotalAmount:    totalAmount,
		Currency:       currency,
		NetAmount:      netAmount,
		MDRPercent:     mdrPercent,
		CompletedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectSettlementCompleted,
		events.SubjectSettlementCompleted,
		merchantID, "Settlement",
		merchantID, "",
		payload,
	)
	if err != nil {
		return fmt.Errorf("st publisher: publish SettlementCompleted: %w", err)
	}
	return nil
}
