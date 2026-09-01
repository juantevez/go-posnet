// Package port define los puertos de entrada del BC Terminal Gateway.
package port

import (
	"context"

	"github.com/juantevez/go-posnet/pkg/domain"
)

// SessionService es el puerto de entrada principal.
// El adaptador WebSocket lo llama al recibir mensajes de los terminales.
type SessionService interface {
	// CreateSession inicia una nueva sesión de pago cuando el cajero ingresa el monto.
	CreateSession(ctx context.Context, cmd CreateSessionCommand) (*SessionCreatedResult, error)

	// ProcessPayment procesa el mensaje ISO 8583 recibido del terminal.
	// Traduce el mensaje al evento TransactionReceived y lo publica a NATS.
	ProcessPayment(ctx context.Context, cmd ProcessPaymentCommand) error

	// RequestReversal solicita la anulación de una transacción aprobada.
	RequestReversal(ctx context.Context, cmd RequestReversalCommand) error

	// CancelSession cancela una sesión activa por acción del cajero.
	CancelSession(ctx context.Context, cmd CancelSessionCommand) error

	// RequestBatchClose solicita el cierre de lote del terminal.
	RequestBatchClose(ctx context.Context, cmd RequestBatchCloseCommand) error
}

// AuthResultService recibe los resultados de autorización desde NATS
// y los entrega al terminal vía WebSocket.
type AuthResultService interface {
	// ApplyApproval procesa el evento AuthorizationApproved y notifica al terminal.
	ApplyApproval(ctx context.Context, cmd ApplyApprovalCommand) error

	// ApplyRejection procesa el evento AuthorizationRejected y notifica al terminal.
	ApplyRejection(ctx context.Context, cmd ApplyRejectionCommand) error
}

// ─── Commands ────────────────────────────────────────────────────────────────

type CreateSessionCommand struct {
	TerminalID     string
	MerchantID     string
	AmountCents    int64
	Currency       string
	STAN           int
	PaymentChannel string // QR | NFC | APPLE_PAY | GOOGLE_PAY | MAGSTRIPE
}

type ProcessPaymentCommand struct {
	EventID       string
	TransactionID string // ID de la sesión activa
	ISO8583Raw    []byte
	EMVDataBase64 string
	CardLast4     string
	CardNetwork   string
	CardToken     string // HMAC del PAN derivado en el borde; vacío si no se emite
}

type RequestReversalCommand struct {
	EventID               string
	OriginalTransactionID string
	TerminalID            string
}

type CancelSessionCommand struct {
	TransactionID string
	TerminalID    string
}

type RequestBatchCloseCommand struct {
	EventID        string
	TerminalID     string
	MerchantID     string
	BatchDate      string
	TerminalCount  int
	TerminalAmount int64
	Currency       string
}

type ApplyApprovalCommand struct {
	EventID       string
	TransactionID string
	TerminalID    string
	AuthCode      string
	AmountCents   int64
	Currency      string
	CardLast4     string
	CardNetwork   string
	AuthorizedAt  string
}

type ApplyRejectionCommand struct {
	EventID         string
	TransactionID   string
	TerminalID      string
	RejectionCode   string
	RejectionReason string
	IsRetryable     bool
	CaptureCard     bool // "pick-up card": el terminal debe retener la tarjeta
	Source          string
}

// ─── Results ─────────────────────────────────────────────────────────────────

type SessionCreatedResult struct {
	TransactionID string
	ExpiresAt     string // RFC3339 UTC
	TTLSeconds    int
	Channel       string
	Amount        domain.Money
}
