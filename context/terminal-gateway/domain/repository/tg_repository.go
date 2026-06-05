// Package repository define los puertos de salida del BC Terminal Gateway.
package repository

import (
	"context"

	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/entity"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// PaymentSessionRepository persiste y recupera sesiones de pago activas.
type PaymentSessionRepository interface {
	// Save persiste una sesión nueva o actualiza una existente (UPSERT).
	Save(ctx context.Context, session *aggregate.PaymentSession) error

	// FindByID recupera una sesión por su TransactionID.
	FindByID(ctx context.Context, id domain.TransactionID) (*aggregate.PaymentSession, error)

	// FindActiveByTerminal recupera la sesión activa de un terminal.
	// Retorna nil si el terminal no tiene sesión activa (estado AWAITING o PROCESSING).
	FindActiveByTerminal(ctx context.Context, terminalID domain.TerminalID) (*aggregate.PaymentSession, error)

	// DeleteExpired elimina sesiones expiradas para mantener la tabla limpia.
	// Llamado por un job periódico, no en el flujo crítico.
	DeleteExpired(ctx context.Context) (int64, error)
}

// TerminalRepository persiste y recupera terminales registrados.
type TerminalRepository interface {
	// FindByID recupera un terminal por su ID.
	FindByID(ctx context.Context, id domain.TerminalID) (*entity.Terminal, error)

	// FindByCertificateCN recupera un terminal por el CN de su certificado mTLS.
	// Usado durante el handshake de autenticación WebSocket.
	FindByCertificateCN(ctx context.Context, cn string) (*entity.Terminal, error)

	// Save persiste un terminal nuevo o actualiza uno existente.
	Save(ctx context.Context, terminal *entity.Terminal) error
}
