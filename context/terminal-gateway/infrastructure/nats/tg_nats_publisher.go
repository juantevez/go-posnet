// Package nats contiene los adaptadores NATS del BC Terminal Gateway.
package nats

import (
	"context"
	"fmt"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/domain"
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

// PublishTransactionReceived publica al stream POSNET_TRANSACTIONS.
// Consumido por: Authorization BC — dispara la Saga de autorización.
func (p *EventPublisher) PublishTransactionReceived(
	ctx context.Context,
	session *aggregate.PaymentSession,
	iso8583Raw []byte,
	emvDataBase64 string,
) error {
	payload := events.TransactionReceivedPayload{
		TransactionID: session.ID().String(),
		TerminalID:    session.TerminalID().String(),
		MerchantID:    session.MerchantID().String(),
		AmountCents:   session.Amount().Cents(),
		Currency:      session.Amount().Currency().String(),
		STAN:          session.STAN().Value(),
		EntryMode:     session.Channel().ToEntryMode(),
		CardNetwork:   "", // Completado por el adaptador ISO 8583 al parsear el mensaje
		CardLast4:     "", // Ídem
		EMVDataBase64: emvDataBase64,
		ISO8583Raw:    iso8583Raw,
		ReceivedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectTransactionReceived,
		events.SubjectTransactionReceived,
		session.ID().String(),
		"PaymentSession",
		session.ID().String(),
		"",
		payload,
	)
	if err != nil {
		return fmt.Errorf("tg publisher: publish TransactionReceived: %w", err)
	}
	return nil
}

// PublishReversalRequested publica al stream POSNET_TRANSACTIONS.
// Consumido por: Authorization BC.
func (p *EventPublisher) PublishReversalRequested(
	ctx context.Context,
	originalTxID domain.TransactionID,
	session *aggregate.PaymentSession,
) error {
	payload := events.ReversalRequestedPayload{
		OriginalTransactionID: originalTxID.String(),
		TerminalID:            session.TerminalID().String(),
		MerchantID:            session.MerchantID().String(),
		AmountCents:           session.Amount().Cents(),
		Currency:              session.Amount().Currency().String(),
		OriginalAuthCode:      session.AuthCode(),
		RequestedAt:           time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectReversalRequested,
		events.SubjectReversalRequested,
		originalTxID.String(),
		"PaymentSession",
		originalTxID.String(),
		"",
		payload,
	)
	if err != nil {
		return fmt.Errorf("tg publisher: publish ReversalRequested: %w", err)
	}
	return nil
}

// PublishBatchCloseRequested publica al stream POSNET_TRANSACTIONS.
// Consumido por: Settlement BC.
func (p *EventPublisher) PublishBatchCloseRequested(
	ctx context.Context,
	terminalID domain.TerminalID,
	merchantID domain.MerchantID,
	terminalCount int,
	terminalAmount int64,
	currency string,
) error {
	payload := events.BatchCloseRequestedPayload{
		TerminalID:     terminalID.String(),
		MerchantID:     merchantID.String(),
		BatchDate:      time.Now().UTC().Format("2006-01-02"),
		TerminalCount:  terminalCount,
		TerminalAmount: terminalAmount,
		Currency:       currency,
		RequestedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	_, err := p.pub.Publish(ctx,
		events.SubjectBatchCloseRequested,
		events.SubjectBatchCloseRequested,
		terminalID.String(),
		"Terminal",
		terminalID.String(),
		"",
		payload,
	)
	if err != nil {
		return fmt.Errorf("tg publisher: publish BatchCloseRequested: %w", err)
	}
	return nil
}
