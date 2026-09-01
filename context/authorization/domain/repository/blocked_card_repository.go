package repository

import (
	"context"

	"github.com/juantevez/go-posnet/pkg/domain"
)

// BlockedCardRepository es el puerto de salida hacia la blocklist de tarjetas.
//
// La blocklist se indexa por CardToken —el HMAC del PAN derivado en el borde—
// porque el PAN completo nunca llega al backend Go. Indexarla por last4 + red
// sería inaceptable: colisionaría con ~1 de cada 10.000 tarjetas legítimas.
type BlockedCardRepository interface {
	// IsBlocked indica si la tarjeta está en la blocklist.
	// Un token cero (transacción sin token) nunca está bloqueado.
	IsBlocked(ctx context.Context, token domain.CardToken) (bool, error)

	// Block agrega la tarjeta a la blocklist de forma idempotente:
	// re-bloquear una tarjeta ya bloqueada no es un error y conserva
	// el motivo y la transacción originales.
	Block(ctx context.Context, token domain.CardToken, reason string, sourceTxID domain.TransactionID) error
}
