package events

import (
	"fmt"
	"reflect"
)

// EventRegistry mapea event_type string → reflect.Type del payload concreto.
// Permite al subscriber deserializar dinámicamente sin un switch manual.
var EventRegistry = map[string]reflect.Type{
	SubjectTransactionReceived:    reflect.TypeOf(TransactionReceivedPayload{}),
	SubjectReversalRequested:      reflect.TypeOf(ReversalRequestedPayload{}),
	SubjectBatchCloseRequested:    reflect.TypeOf(BatchCloseRequestedPayload{}),
	SubjectFraudCheckRequested:    reflect.TypeOf(FraudCheckRequestedPayload{}),
	SubjectFraudScoreCalculated:   reflect.TypeOf(FraudScoreCalculatedPayload{}),
	SubjectAuthApproved:           reflect.TypeOf(AuthorizationApprovedPayload{}),
	SubjectAuthRejected:           reflect.TypeOf(AuthorizationRejectedPayload{}),
	SubjectReversalCompleted:      reflect.TypeOf(ReversalCompletedPayload{}),
	SubjectBatchClosed:            reflect.TypeOf(BatchClosedPayload{}),
	SubjectSettlementCompleted:    reflect.TypeOf(SettlementCompletedPayload{}),
	SubjectNotificationDispatched: reflect.TypeOf(NotificationDispatchedPayload{}),
}

// ResolvePayloadType devuelve el tipo Go del payload para un eventType dado.
// Devuelve error si el eventType no está registrado.
func ResolvePayloadType(eventType string) (reflect.Type, error) {
	t, ok := EventRegistry[eventType]
	if !ok {
		return nil, fmt.Errorf("events: unknown event_type %q — not registered", eventType)
	}
	return t, nil
}
