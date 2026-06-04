package events

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DomainEvent es el envelope que envuelve cualquier evento de dominio.
// El publisher serializa el DomainEvent completo a JSON.
// El consumer deserializa el envelope, inspecciona EventType,
// y luego deserializa Data al tipo concreto usando Unwrap[T].
type DomainEvent struct {
	EventID       string          `json:"event_id"`       // UUID v4 — clave de idempotencia
	EventType     string          `json:"event_type"`     // ej: "posnet.auth.approved.v1"
	AggregateID   string          `json:"aggregate_id"`   // ID del aggregate que originó el evento
	AggregateType string          `json:"aggregate_type"` // ej: "Transaction"
	CorrelationID string          `json:"correlation_id"` // TransactionID — trazabilidad E2E
	CausationID   string          `json:"causation_id"`   // EventID del evento que causó éste
	OccurredAt    time.Time       `json:"occurred_at"`    // Timestamp UTC — inmutable
	SchemaVersion int             `json:"schema_version"` // Para evolución de esquema
	Data          json.RawMessage `json:"data"`           // Payload específico del evento
}

// Wrap serializa un payload concreto dentro de un DomainEvent.
// Genera automáticamente un nuevo EventID.
func Wrap(
	eventType string,
	aggregateID string,
	aggregateType string,
	correlationID string,
	causationID string,
	payload any,
) (DomainEvent, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return DomainEvent{}, fmt.Errorf("events.Wrap: marshal payload: %w", err)
	}
	return DomainEvent{
		EventID:       uuid.NewString(),
		EventType:     eventType,
		AggregateID:   aggregateID,
		AggregateType: aggregateType,
		CorrelationID: correlationID,
		CausationID:   causationID,
		OccurredAt:    time.Now().UTC(),
		SchemaVersion: 1,
		Data:          data,
	}, nil
}

// Unwrap deserializa el campo Data del envelope en el tipo destino T.
// Uso: payload, err := events.Unwrap[AuthorizationApprovedPayload](envelope)
func Unwrap[T any](e DomainEvent) (T, error) {
	var payload T
	if err := json.Unmarshal(e.Data, &payload); err != nil {
		return payload, fmt.Errorf("events.Unwrap[%T]: unmarshal data: %w", payload, err)
	}
	return payload, nil
}

// MarshalJSON serializa el DomainEvent completo a JSON.
func (e DomainEvent) Marshal() ([]byte, error) {
	return json.Marshal(e)
}

// UnmarshalEnvelope deserializa un JSON en un DomainEvent sin tocar Data.
func UnmarshalEnvelope(data []byte) (DomainEvent, error) {
	var e DomainEvent
	if err := json.Unmarshal(data, &e); err != nil {
		return DomainEvent{}, fmt.Errorf("events.UnmarshalEnvelope: %w", err)
	}
	return e, nil
}
