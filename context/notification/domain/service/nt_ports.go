// Package service contiene los puertos de salida del BC Notification.
package service

import (
	"context"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
)

// TerminalNotifier es el puerto de salida hacia Terminal Gateway vía gRPC.
// Entrega el comprobante al WebSocket del terminal.
type TerminalNotifier interface {
	// SendReceipt envía el comprobante al terminal.
	// Retorna false + motivo si el terminal no tiene sesión activa.
	SendReceipt(ctx context.Context, n *aggregate.Notification) (delivered bool, reason string, err error)
}

// WebhookDispatcher es el puerto de salida hacia el endpoint HTTP del comercio.
type WebhookDispatcher interface {
	// Dispatch envía el payload de la notificación al endpoint configurado.
	// Retorna el HTTP status code recibido y un error si hubo fallo de red.
	Dispatch(ctx context.Context, n *aggregate.Notification) (httpStatus int, err error)
}

// EventPublisher es el puerto de salida hacia NATS JetStream.
type EventPublisher interface {
	// PublishDispatched publica el evento de auditoría NotificationDispatched.
	PublishDispatched(ctx context.Context, n *aggregate.Notification) error
}

// ReceiptBuilder construye el ReceiptPayload a partir de los datos del evento.
// Es un Domain Service — lógica de construcción que no pertenece al aggregate.
type ReceiptBuilder interface {
	// BuildFromApproval construye un comprobante de aprobación.
	BuildFromApproval(
		transactionID, terminalID, merchantID string,
		amountCents int64, currency string,
		authCode, cardLast4, cardNetwork, entryMode, authorizedAt string,
	) (interface{}, error)

	// BuildFromRejection construye un comprobante de rechazo.
	BuildFromRejection(
		transactionID, terminalID, merchantID string,
		amountCents int64, currency string,
		rejectionCode, rejectionReason, cardLast4, cardNetwork, entryMode, rejectedAt string,
	) (interface{}, error)
}
