// Package service contiene los puertos de salida del BC Terminal Gateway.
package service

import (
	"context"

	"github.com/tu-org/posnet-backend/context/terminal-gateway/domain/aggregate"
	"github.com/tu-org/posnet-backend/pkg/domain"
)

// TerminalNotifier es el puerto de salida hacia el WebSocket del terminal.
// El adaptador WebSocket lo implementa manteniendo un mapa de conexiones activas.
type TerminalNotifier interface {
	// NotifyResult envía el resultado de la transacción al terminal vía WebSocket.
	// Si el terminal no tiene sesión WebSocket activa, retorna ErrTerminalNotConnected.
	NotifyResult(ctx context.Context, session *aggregate.PaymentSession) error

	// NotifySessionCreated envía el payload de la sesión (QR o instrucción NFC)
	// al terminal para que lo muestre en pantalla.
	NotifySessionCreated(ctx context.Context, session *aggregate.PaymentSession) error

	// NotifySessionExpired avisa al terminal que el TTL de la sesión venció.
	NotifySessionExpired(ctx context.Context, terminalID domain.TerminalID, transactionID domain.TransactionID) error
}

// EventPublisher es el puerto de salida hacia NATS JetStream.
type EventPublisher interface {
	// PublishTransactionReceived publica el evento al stream POSNET_TRANSACTIONS.
	// Consumido por: Authorization BC.
	PublishTransactionReceived(ctx context.Context, session *aggregate.PaymentSession, iso8583Raw []byte, emvDataBase64 string) error

	// PublishReversalRequested publica la solicitud de anulación.
	// Consumido por: Authorization BC.
	PublishReversalRequested(ctx context.Context, originalTxID domain.TransactionID, session *aggregate.PaymentSession) error

	// PublishBatchCloseRequested publica el cierre de lote solicitado por el terminal.
	// Consumido por: Settlement BC.
	PublishBatchCloseRequested(ctx context.Context, terminalID domain.TerminalID, merchantID domain.MerchantID, terminalCount int, terminalAmount int64, currency string) error
}
