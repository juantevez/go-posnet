package natsutil

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juantevez/posnet-backend/pkg/events"
	"github.com/juantevez/posnet-backend/pkg/observability"
	"github.com/nats-io/nats.go"
)

// Publisher publica eventos de dominio a JetStream con envelope automático
// y propagación de trace context en los headers del mensaje.
type Publisher struct {
	js nats.JetStreamContext
}

// NewPublisher crea un Publisher a partir de un JetStreamContext.
func NewPublisher(js nats.JetStreamContext) *Publisher {
	return &Publisher{js: js}
}

// Publish serializa el payload en un DomainEvent envelope y lo publica.
// Inyecta el trace context en los headers del mensaje NATS.
// Retorna el sequence number asignado por JetStream.
func (p *Publisher) Publish(
	ctx context.Context,
	subject string,
	eventType string,
	aggregateID string,
	aggregateType string,
	correlationID string,
	causationID string,
	payload any,
) (uint64, error) {
	envelope, err := events.Wrap(eventType, aggregateID, aggregateType, correlationID, causationID, payload)
	if err != nil {
		return 0, fmt.Errorf("publisher: wrap event %q: %w", eventType, err)
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return 0, fmt.Errorf("publisher: marshal envelope: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
		Header:  make(nats.Header),
	}

	// Inyectar trace context (TraceID, SpanID) en los headers.
	observability.InjectTraceContext(ctx, msg)

	// MsgID para deduplicación exactly-once de JetStream.
	msg.Header.Set(nats.MsgIdHdr, envelope.EventID)

	ack, err := p.js.PublishMsg(msg)
	if err != nil {
		return 0, fmt.Errorf("publisher: publish to %q: %w", subject, err)
	}

	observability.AddEvent(ctx, "nats.publish") // atributos del span para trazabilidad

	return ack.Sequence, nil
}
