package postgres

import (
	"context"
	"fmt"

	"github.com/juantevez/go-posnet/pkg/domain"
)

// BlockedCardRepo implementa repository.BlockedCardRepository usando pgx/v5.
type BlockedCardRepo struct {
	pool pgxPool
}

// NewBlockedCardRepo construye el repositorio con el pool de conexiones.
func NewBlockedCardRepo(pool pgxPool) *BlockedCardRepo {
	return &BlockedCardRepo{pool: pool}
}

// IsBlocked consulta la blocklist por token de tarjeta.
//
// Una transacción sin token no puede ser bloqueada: se corta antes de tocar
// la base para no gastar un round-trip por cada terminal que todavía no emite
// el token, y para que la ausencia de token nunca se confunda con una clave.
func (r *BlockedCardRepo) IsBlocked(ctx context.Context, token domain.CardToken) (bool, error) {
	if token.IsZero() {
		return false, nil
	}

	const query = `
		SELECT EXISTS (
			SELECT 1 FROM pn_authorization.blocked_cards WHERE card_token = $1
		)`

	var blocked bool
	if err := r.pool.QueryRow(ctx, query, token.String()).Scan(&blocked); err != nil {
		return false, fmt.Errorf("blocked_cards: is blocked: %w", err)
	}
	return blocked, nil
}

// Block agrega la tarjeta a la blocklist.
//
// ON CONFLICT DO NOTHING la hace idempotente y preserva el primer bloqueo:
// si la misma tarjeta robada se reintenta en varios terminales, queda
// registrada la transacción que la detectó primero.
func (r *BlockedCardRepo) Block(
	ctx context.Context,
	token domain.CardToken,
	reason string,
	sourceTxID domain.TransactionID,
) error {
	if token.IsZero() {
		return fmt.Errorf("blocked_cards: cannot block a card without token")
	}

	const query = `
		INSERT INTO pn_authorization.blocked_cards (card_token, reason, source_transaction_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (card_token) DO NOTHING`

	if _, err := r.pool.Exec(ctx, query, token.String(), reason, sourceTxID.String()); err != nil {
		return fmt.Errorf("blocked_cards: block: %w", err)
	}
	return nil
}
