package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/juantevez/go-posnet/context/authorization/application/command"
	"github.com/juantevez/go-posnet/context/authorization/application/port"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/observability"
	natsclient "github.com/nats-io/nats.go"
)

// Subscriber registra y gestiona los durable consumers del BC Authorization.
type Subscriber struct {
	js      natsclient.JetStreamContext
	handler *command.AuthorizationHandler // tipo concreto — elimina la interface ambigua
	log     *slog.Logger
}

// NewSubscriber construye el Subscriber con el handler concreto.
// Se usa el tipo concreto *command.AuthorizationHandler en lugar de una
// interface local para evitar desincronías de firmas entre capas.
func NewSubscriber(js natsclient.JetStreamContext, handler *command.AuthorizationHandler) *Subscriber {
	return &Subscriber{
		js:      js,
		handler: handler,
		log:     slog.Default().With(slog.String("component", "authorization.subscriber")),
	}
}

// Subscribe registra los 3 consumers del BC Authorization.
func (s *Subscriber) Subscribe() error {
	consumers := []struct {
		subject  string
		durable  string
		handler  natsclient.MsgHandler
		maxDeliv int
	}{
		{
			subject:  events.SubjectTransactionReceived,
			durable:  "auth-txn-receiver",
			handler:  s.handleTransactionReceived,
			maxDeliv: 5,
		},
		{
			subject:  events.SubjectFraudScoreCalculated,
			durable:  "auth-fraud-score-consumer",
			handler:  s.handleFraudScoreCalculated,
			maxDeliv: 5,
		},
		{
			subject:  events.SubjectReversalRequested,
			durable:  "auth-reversal-processor",
			handler:  s.handleReversalRequested,
			maxDeliv: 3,
		},
	}

	for _, c := range consumers {
		_, err := s.js.QueueSubscribe(
			c.subject,
			c.durable,
			c.handler,
			natsclient.Durable(c.durable),
			natsclient.AckExplicit(),
			natsclient.MaxDeliver(c.maxDeliv),
			natsclient.AckWait(30*time.Second),
			natsclient.DeliverNew(),
		)
		if err != nil {
			return fmt.Errorf("subscriber: register consumer %q: %w", c.durable, err)
		}
		s.log.Info("consumer registered", slog.String("durable", c.durable))
	}
	return nil
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func (s *Subscriber) handleTransactionReceived(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleTransactionReceived")
	defer span.End()

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unmarshal envelope", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	payload, err := events.Unwrap[events.TransactionReceivedPayload](envelope)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unwrap TransactionReceived",
			slog.String("event_id", envelope.EventID),
			slog.String("error", err.Error()),
		)
		_ = msg.Term()
		return
	}

	cmd := port.AuthorizeTransactionCommand{
		EventID:       envelope.EventID,
		TransactionID: payload.TransactionID,
		TerminalID:    payload.TerminalID,
		MerchantID:    payload.MerchantID,
		AmountCents:   payload.AmountCents,
		Currency:      payload.Currency,
		STAN:          payload.STAN,
		EntryMode:     payload.EntryMode,
		CardLast4:     payload.CardLast4,
		CardNetwork:   payload.CardNetwork,
		EMVDataBase64: payload.EMVDataBase64,
		ISO8583Raw:    payload.ISO8583Raw,
		ReceivedAt:    payload.ReceivedAt,
	}

	if err := s.handler.AuthorizeTransaction(ctx, cmd); err != nil {
		s.nak(ctx, msg, err, envelope.EventID)
		return
	}

	_ = msg.Ack()
}

func (s *Subscriber) handleFraudScoreCalculated(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleFraudScoreCalculated")
	defer span.End()

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unmarshal envelope", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	payload, err := events.Unwrap[events.FraudScoreCalculatedPayload](envelope)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unwrap FraudScoreCalculated",
			slog.String("event_id", envelope.EventID),
			slog.String("error", err.Error()),
		)
		_ = msg.Term()
		return
	}

	cmd := port.ApplyFraudScoreCommand{
		EventID:       envelope.EventID,
		TransactionID: payload.TransactionID,
		Score:         payload.Score,
		Decision:      payload.Decision,
		RulesHit:      payload.RulesHit,
	}

	if err := s.handler.ApplyFraudScore(ctx, cmd); err != nil {
		s.nak(ctx, msg, err, envelope.EventID)
		return
	}

	_ = msg.Ack()
}

func (s *Subscriber) handleReversalRequested(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleReversalRequested")
	defer span.End()

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unmarshal envelope", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	payload, err := events.Unwrap[events.ReversalRequestedPayload](envelope)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unwrap ReversalRequested",
			slog.String("event_id", envelope.EventID),
			slog.String("error", err.Error()),
		)
		_ = msg.Term()
		return
	}

	cmd := port.ProcessReversalCommand{
		EventID:               envelope.EventID,
		OriginalTransactionID: payload.OriginalTransactionID,
		TerminalID:            payload.TerminalID,
		MerchantID:            payload.MerchantID,
		AmountCents:           payload.AmountCents,
		Currency:              payload.Currency,
		OriginalAuthCode:      payload.OriginalAuthCode,
	}

	if err := s.handler.ProcessReversal(ctx, cmd); err != nil {
		s.nak(ctx, msg, err, envelope.EventID)
		return
	}

	_ = msg.Ack()
}

// nak decide Nak (reintento) o Term (DLQ) según el tipo de error.
func (s *Subscriber) nak(ctx context.Context, msg *natsclient.Msg, err error, eventID string) {
	observability.RecordError(ctx, err)

	var validationErr *pkgerrors.ValidationError
	var conflictErr *pkgerrors.ConflictError

	if errors.As(err, &validationErr) || errors.As(err, &conflictErr) {
		s.log.ErrorContext(ctx, "permanent error — terminating message",
			slog.String("event_id", eventID),
			slog.String("error", err.Error()),
		)
		_ = msg.Term()
		return
	}

	s.log.WarnContext(ctx, "transient error — nacking for retry",
		slog.String("event_id", eventID),
		slog.String("error", err.Error()),
	)
	_ = msg.Nak()
}
