// Package nats contiene los adaptadores NATS del BC Authorization.
// Publisher publica eventos de dominio como eventos de integración.
// Subscriber consume eventos de otros BCs y los traduce a commands.
package nats

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/domain/aggregate"
	"github.com/juantevez/go-posnet/pkg/domain"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
	natsclient "github.com/nats-io/nats.go"
)

// ─── Publisher ────────────────────────────────────────────────────────────────

// EventPublisher implementa domain/service.EventPublisher usando NATS JetStream.
type EventPublisher struct {
	pub *natsutil.Publisher
}

func NewEventPublisher(pub *natsutil.Publisher) *EventPublisher {
	return &EventPublisher{pub: pub}
}

func (p *EventPublisher) PublishApproved(ctx context.Context, tx *aggregate.Transaction) error {
	payload := events.AuthorizationApprovedPayload{
		TransactionID: tx.ID().String(),
		TerminalID:    tx.TerminalID().String(),
		MerchantID:    tx.MerchantID().String(),
		AuthCode:      tx.AuthCode().String(),
		AmountCents:   tx.Amount().Cents(),
		Currency:      tx.Amount().Currency().String(),
		CardLast4:     tx.PAN().Last4(),
		CardNetwork:   string(tx.PAN().Network()),
		EntryMode:     tx.EntryMode().String(),
		FraudScore:    tx.FraudDecision().Score,
		AuthorizedAt:  tx.AuthorizedAt().Format(time.RFC3339),
	}
	_, err := p.pub.Publish(ctx,
		events.SubjectAuthApproved,
		events.SubjectAuthApproved,
		tx.ID().String(), "Transaction",
		tx.ID().String(), "",
		payload,
	)
	return err
}

func (p *EventPublisher) PublishRejected(ctx context.Context, tx *aggregate.Transaction) error {
	rc := tx.RejectionCode()
	payload := events.AuthorizationRejectedPayload{
		TransactionID:   tx.ID().String(),
		TerminalID:      tx.TerminalID().String(),
		MerchantID:      tx.MerchantID().String(),
		RejectionCode:   rc.Code(),
		RejectionReason: rc.Description(),
		IsRetryable:     rc.IsRetryable(),
		Source:          string(rc.Source()),
		RejectedAt:      time.Now().UTC().Format(time.RFC3339),
	}
	_, err := p.pub.Publish(ctx,
		events.SubjectAuthRejected,
		events.SubjectAuthRejected,
		tx.ID().String(), "Transaction",
		tx.ID().String(), "",
		payload,
	)
	return err
}

func (p *EventPublisher) PublishFraudCheckRequested(ctx context.Context, tx *aggregate.Transaction) error {
	payload := events.FraudCheckRequestedPayload{
		TransactionID: tx.ID().String(),
		TerminalID:    tx.TerminalID().String(),
		MerchantID:    tx.MerchantID().String(),
		AmountCents:   tx.Amount().Cents(),
		Currency:      tx.Amount().Currency().String(),
		CardNetwork:   string(tx.PAN().Network()),
		EntryMode:     tx.EntryMode().String(),
		OccurredAt:    time.Now().UTC().Format(time.RFC3339),
	}
	_, err := p.pub.Publish(ctx,
		events.SubjectFraudCheckRequested,
		events.SubjectFraudCheckRequested,
		tx.ID().String(), "Transaction",
		tx.ID().String(), "",
		payload,
	)
	return err
}

func (p *EventPublisher) PublishReversalCompleted(ctx context.Context, txID domain.TransactionID, tx *aggregate.Transaction) error {
	payload := events.ReversalCompletedPayload{
		OriginalTransactionID: txID.String(),
		TerminalID:            tx.TerminalID().String(),
		MerchantID:            tx.MerchantID().String(),
		AmountCents:           tx.Amount().Cents(),
		Currency:              tx.Amount().Currency().String(),
		CompletedAt:           time.Now().UTC().Format(time.RFC3339),
	}
	_, err := p.pub.Publish(ctx,
		events.SubjectReversalCompleted,
		events.SubjectReversalCompleted,
		txID.String(), "Transaction",
		txID.String(), "",
		payload,
	)
	return err
}

// ─── Subscriber ───────────────────────────────────────────────────────────────

// Subscriber consume eventos de NATS y los traduce a commands del BC.
type Subscriber struct {
	js          natsclient.JetStreamContext
	authHandler AuthorizationService
	log         *slog.Logger
}

// AuthorizationService es la interfaz local que el subscriber necesita del handler.
type AuthorizationService interface {
	AuthorizeTransaction(ctx context.Context, cmd interface{}) error
	ApplyFraudScore(ctx context.Context, cmd interface{}) error
	ProcessReversal(ctx context.Context, cmd interface{}) error
}

func NewSubscriber(js natsclient.JetStreamContext, authHandler AuthorizationService) *Subscriber {
	return &Subscriber{
		js:          js,
		authHandler: authHandler,
		log:         slog.Default().With(slog.String("component", "auth.subscriber")),
	}
}

// Subscribe registra todos los consumers del BC Authorization.
func (s *Subscriber) Subscribe() error {
	// Consumer: TransactionReceived → AuthorizeTransaction
	if _, err := s.js.QueueSubscribe(
		events.SubjectTransactionReceived,
		"auth-txn-receiver",
		s.handleTransactionReceived,
		natsclient.Durable("auth-txn-receiver"),
		natsclient.AckExplicit(),
		natsclient.MaxDeliver(5),
		natsclient.AckWait(30*time.Second),
	); err != nil {
		return fmt.Errorf("subscriber: subscribe TransactionReceived: %w", err)
	}

	// Consumer: FraudScoreCalculated → ApplyFraudScore
	if _, err := s.js.QueueSubscribe(
		events.SubjectFraudScoreCalculated,
		"auth-fraud-score-consumer",
		s.handleFraudScoreCalculated,
		natsclient.Durable("auth-fraud-score-consumer"),
		natsclient.AckExplicit(),
		natsclient.MaxDeliver(5),
		natsclient.AckWait(30*time.Second),
	); err != nil {
		return fmt.Errorf("subscriber: subscribe FraudScoreCalculated: %w", err)
	}

	// Consumer: ReversalRequested → ProcessReversal
	if _, err := s.js.QueueSubscribe(
		events.SubjectReversalRequested,
		"auth-reversal-processor",
		s.handleReversalRequested,
		natsclient.Durable("auth-reversal-processor"),
		natsclient.AckExplicit(),
		natsclient.MaxDeliver(3),
		natsclient.AckWait(30*time.Second),
	); err != nil {
		return fmt.Errorf("subscriber: subscribe ReversalRequested: %w", err)
	}

	s.log.Info("all consumers registered successfully")
	return nil
}

func (s *Subscriber) handleTransactionReceived(msg *natsclient.Msg) {
	// Extraer trace context de los headers del mensaje
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleTransactionReceived")
	defer span.End()

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.Error("failed to unmarshal envelope", slog.String("error", err.Error()))
		_ = msg.Nak() // Reintento con backoff
		return
	}

	payload, err := events.Unwrap[events.TransactionReceivedPayload](envelope)
	if err != nil {
		s.log.Error("failed to unwrap TransactionReceived payload", slog.String("error", err.Error()))
		_ = msg.Term() // Mensaje malformado — no reintentar, ir a DLQ
		return
	}

	// El handler real recibe el command tipado
	// (simplificado aquí para evitar imports circulares en el ejemplo)
	s.log.Info("processing TransactionReceived",
		slog.String("transaction_id", payload.TransactionID),
		slog.String("terminal_id", payload.TerminalID),
	)

	// En la implementación real: authHandler.AuthorizeTransaction(ctx, cmd)
	// Si falla con error transitorio → Nak(); si falla permanentemente → Term()
	_ = msg.Ack()
}

func (s *Subscriber) handleFraudScoreCalculated(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleFraudScoreCalculated")
	defer span.End()
	_ = ctx

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.Error("failed to unmarshal FraudScoreCalculated", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	payload, err := events.Unwrap[events.FraudScoreCalculatedPayload](envelope)
	if err != nil {
		s.log.Error("failed to unwrap FraudScoreCalculated payload", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	s.log.Info("processing FraudScoreCalculated",
		slog.String("transaction_id", payload.TransactionID),
		slog.Int("score", payload.Score),
		slog.String("decision", payload.Decision),
	)

	_ = msg.Ack()
}

func (s *Subscriber) handleReversalRequested(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleReversalRequested")
	defer span.End()
	_ = ctx

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.Error("failed to unmarshal ReversalRequested", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	payload, err := events.Unwrap[events.ReversalRequestedPayload](envelope)
	if err != nil {
		s.log.Error("failed to unwrap ReversalRequested payload", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	s.log.Info("processing ReversalRequested",
		slog.String("original_tx_id", payload.OriginalTransactionID),
	)

	_ = msg.Ack()
}
