package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/juantevez/go-posnet/context/terminal-gateway/application/command"
	"github.com/juantevez/go-posnet/context/terminal-gateway/application/port"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/observability"
	natsclient "github.com/nats-io/nats.go"
)

// Subscriber registra y gestiona los durable consumers del BC Terminal Gateway.
// Consume los resultados de autorización publicados por el BC Authorization
// y los entrega al terminal vía WebSocket.
type TG_Subscriber struct {
	js      natsclient.JetStreamContext
	handler *command.SessionHandler
	log     *slog.Logger
}

func NewSubscriber(js natsclient.JetStreamContext, handler *command.SessionHandler) *TG_Subscriber {
	return &TG_Subscriber{
		js:      js,
		handler: handler,
		log:     slog.Default().With(slog.String("component", "terminal-gateway.subscriber")),
	}
}

// Subscribe registra el consumer del BC Terminal Gateway.
// Consume posnet.auth.> para recibir aprobaciones y rechazos.
func (s *TG_Subscriber) Subscribe() error {
	_, err := s.js.QueueSubscribe(
		"posnet.auth.>",
		"gateway-auth-consumer",
		countedHandler(s.handleAuthResult),
		natsclient.Durable("gateway-auth-consumer"),
		natsclient.AckExplicit(),
		natsclient.MaxDeliver(3),
		natsclient.AckWait(30*time.Second),
		natsclient.DeliverNew(),
	)
	if err != nil {
		return fmt.Errorf("tg subscriber: register gateway-auth-consumer: %w", err)
	}

	s.log.Info("consumer registered", slog.String("durable", "gateway-auth-consumer"))
	return nil
}

// handleAuthResult enruta el evento al handler correcto según el EventType.
func (s *TG_Subscriber) handleAuthResult(msg *natsclient.Msg) {
	ctx := observability.ExtractTraceContext(context.Background(), msg)
	ctx, span := observability.StartSpan(ctx, "subscriber.handleAuthResult")
	defer span.End()

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		s.log.ErrorContext(ctx, "failed to unmarshal envelope", slog.String("error", err.Error()))
		_ = msg.Term()
		return
	}

	var handleErr error

	switch envelope.EventType {
	case events.SubjectAuthApproved:
		handleErr = s.handleApproval(ctx, envelope)
	case events.SubjectAuthRejected:
		handleErr = s.handleRejection(ctx, envelope)
	default:
		// Evento desconocido en este subject — ignorar silenciosamente
		s.log.WarnContext(ctx, "unknown event type — skipping",
			slog.String("event_type", envelope.EventType),
		)
		_ = msg.Ack()
		return
	}

	if handleErr != nil {
		s.nak(ctx, msg, handleErr, envelope.EventID)
		return
	}

	_ = msg.Ack()
}

func (s *TG_Subscriber) handleApproval(ctx context.Context, envelope events.DomainEvent) error {
	payload, err := events.Unwrap[events.AuthorizationApprovedPayload](envelope)
	if err != nil {
		return fmt.Errorf("unwrap AuthorizationApproved: %w", err)
	}

	cmd := port.ApplyApprovalCommand{
		EventID:       envelope.EventID,
		TransactionID: payload.TransactionID,
		TerminalID:    payload.TerminalID,
		AuthCode:      payload.AuthCode,
		AmountCents:   payload.AmountCents,
		Currency:      payload.Currency,
		CardLast4:     payload.CardLast4,
		CardNetwork:   payload.CardNetwork,
		AuthorizedAt:  payload.AuthorizedAt,
	}

	return s.handler.ApplyApproval(ctx, cmd)
}

func (s *TG_Subscriber) handleRejection(ctx context.Context, envelope events.DomainEvent) error {
	payload, err := events.Unwrap[events.AuthorizationRejectedPayload](envelope)
	if err != nil {
		return fmt.Errorf("unwrap AuthorizationRejected: %w", err)
	}

	cmd := port.ApplyRejectionCommand{
		EventID:         envelope.EventID,
		TransactionID:   payload.TransactionID,
		TerminalID:      payload.TerminalID,
		RejectionCode:   payload.RejectionCode,
		RejectionReason: payload.RejectionReason,
		IsRetryable:     payload.IsRetryable,
		CaptureCard:     payload.CaptureCard,
		Source:          payload.Source,
	}

	return s.handler.ApplyRejection(ctx, cmd)
}

func (s *TG_Subscriber) nak(ctx context.Context, msg *natsclient.Msg, err error, eventID string) {
	observability.RecordError(ctx, err)

	var validationErr *pkgerrors.ValidationError
	var conflictErr *pkgerrors.ConflictError

	if errors.As(err, &validationErr) || errors.As(err, &conflictErr) {
		s.log.ErrorContext(ctx, "permanent error — terminating message",
			slog.String("event_id", eventID),
			slog.String("error", err.Error()),
		)
		observability.RecordNATSFailed(ctx, msg.Subject, "validation")
		_ = msg.Term()
		return
	}

	s.log.WarnContext(ctx, "transient error — nacking for retry",
		slog.String("event_id", eventID),
		slog.String("error", err.Error()),
	)
	observability.RecordNATSFailed(ctx, msg.Subject, "transient")
	_ = msg.Nak()
}

// countedHandler envuelve un MsgHandler para contabilizar cada mensaje entregado
// en posnet_nats_messages_processed_total{subject}.
func countedHandler(h natsclient.MsgHandler) natsclient.MsgHandler {
	return func(m *natsclient.Msg) {
		observability.RecordNATSProcessed(context.Background(), m.Subject)
		h(m)
	}
}
