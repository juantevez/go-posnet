// Package postgres contiene el adaptador de PostgreSQL para el BC Authorization.
// Implementa las interfaces de repositorio definidas en domain/repository.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/authorization/domain/aggregate"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

// TransactionRepo implementa repository.TransactionRepository usando pgx/v5.
// Usa sqlc para las queries (código generado en infrastructure/postgres/sqlc/).
// Este archivo contiene solo las queries complejas que sqlc no genera bien.
type TransactionRepo struct {
	pool *pgxpool.Pool
}

// NewTransactionRepo construye el repositorio con el pool de conexiones.
func NewTransactionRepo(pool *pgxpool.Pool) *TransactionRepo {
	return &TransactionRepo{pool: pool}
}

// Save persiste una Transaction en Postgres usando UPSERT.
// Si la transacción ya existe (mismo ID), actualiza todos los campos mutables.
func (r *TransactionRepo) Save(ctx context.Context, tx *aggregate.Transaction) error {
	const query = `
		INSERT INTO authorization.transactions (
			id, terminal_id, merchant_id,
			state, amount_cents, currency,
			pan_last4, card_network, entry_mode,
			stan, auth_code, rejection_code, rejection_source,
			fraud_score, fraud_decision,
			emv_data_b64, iso8583_raw,
			created_at, authorized_at, rejected_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8, $9,
			$10, $11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19, $20
		)
		ON CONFLICT (id) DO UPDATE SET
			state           = EXCLUDED.state,
			auth_code       = EXCLUDED.auth_code,
			rejection_code  = EXCLUDED.rejection_code,
			rejection_source= EXCLUDED.rejection_source,
			fraud_score     = EXCLUDED.fraud_score,
			fraud_decision  = EXCLUDED.fraud_decision,
			authorized_at   = EXCLUDED.authorized_at,
			rejected_at     = EXCLUDED.rejected_at
	`

	var authCode *string
	if tx.AuthCode() != nil {
		s := tx.AuthCode().String()
		authCode = &s
	}

	var rejCode, rejSource *string
	if tx.RejectionCode() != nil {
		c := tx.RejectionCode().Code()
		s := string(tx.RejectionCode().Source())
		rejCode = &c
		rejSource = &s
	}

	var fraudScore *int
	var fraudDecision *string
	if !tx.FraudDecision().IsZero() {
		fs := tx.FraudDecision().Score
		fd := tx.FraudDecision().Decision
		fraudScore = &fs
		fraudDecision = &fd
	}

	_, err := r.pool.Exec(ctx, query,
		tx.ID().String(),
		tx.TerminalID().String(),
		tx.MerchantID().String(),
		tx.State().String(),
		tx.Amount().Cents(),
		tx.Amount().Currency().String(),
		tx.PAN().Last4(),
		string(tx.PAN().Network()),
		tx.EntryMode().String(),
		tx.STAN().Value(),
		authCode,
		rejCode,
		rejSource,
		fraudScore,
		fraudDecision,
		tx.EMVDataBase64(),
		tx.ISO8583Raw(),
		tx.ReceivedAt(),
		tx.AuthorizedAt(),
		tx.RejectedAt(),
	)
	if err != nil {
		return fmt.Errorf("TransactionRepo.Save: exec upsert: %w", err)
	}
	return nil
}

// FindByID recupera una Transaction por su ID.
func (r *TransactionRepo) FindByID(ctx context.Context, id domain.TransactionID) (*aggregate.Transaction, error) {
	const query = `
		SELECT
			id, terminal_id, merchant_id,
			state, amount_cents, currency,
			pan_last4, card_network, entry_mode,
			stan, auth_code, rejection_code, rejection_source,
			fraud_score, fraud_decision, fraud_rules_hit,
			emv_data_b64, iso8583_raw,
			created_at, authorized_at, rejected_at
		FROM authorization.transactions
		WHERE id = $1
	`

	row := r.pool.QueryRow(ctx, query, id.String())
	return scanTransaction(row)
}

// FindBySTAN recupera una transacción por STAN y terminal en un día dado.
func (r *TransactionRepo) FindBySTAN(
	ctx context.Context,
	terminalID domain.TerminalID,
	stan domain.STAN,
	date time.Time,
) (*aggregate.Transaction, error) {
	const query = `
		SELECT
			id, terminal_id, merchant_id,
			state, amount_cents, currency,
			pan_last4, card_network, entry_mode,
			stan, auth_code, rejection_code, rejection_source,
			fraud_score, fraud_decision, fraud_rules_hit,
			emv_data_b64, iso8583_raw,
			created_at, authorized_at, rejected_at
		FROM authorization.transactions
		WHERE terminal_id = $1
		  AND stan = $2
		  AND created_at::date = $3::date
		LIMIT 1
	`

	row := r.pool.QueryRow(ctx, query, terminalID.String(), stan.Value(), date)
	tx, err := scanTransaction(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No encontrado no es un error en este caso
		}
		return nil, err
	}
	return tx, nil
}

// UpdateState actualiza solo el estado de una transacción.
func (r *TransactionRepo) UpdateState(
	ctx context.Context,
	id domain.TransactionID,
	state valueobject.TransactionState,
) error {
	const query = `UPDATE authorization.transactions SET state = $1 WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, state.String(), id.String())
	if err != nil {
		return fmt.Errorf("TransactionRepo.UpdateState: %w", err)
	}
	return nil
}

// ExistsByID verifica si una transacción con ese ID ya existe.
func (r *TransactionRepo) ExistsByID(ctx context.Context, id domain.TransactionID) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM authorization.transactions WHERE id = $1)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, id.String()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("TransactionRepo.ExistsByID: %w", err)
	}
	return exists, nil
}

// ─── Scanner ──────────────────────────────────────────────────────────────────

// txRow es la estructura que mapea la fila de Postgres al aggregate.
type txRow struct {
	ID              string
	TerminalID      string
	MerchantID      string
	State           string
	AmountCents     int64
	Currency        string
	PanLast4        string
	CardNetwork     string
	EntryMode       string
	Stan            int
	AuthCode        *string
	RejectionCode   *string
	RejectionSource *string
	FraudScore      *int
	FraudDecision   *string
	FraudRulesHit   []byte // JSONB
	EMVDataB64      string
	ISO8583Raw      []byte
	CreatedAt       time.Time
	AuthorizedAt    *time.Time
	RejectedAt      *time.Time
}

func scanTransaction(row pgx.Row) (*aggregate.Transaction, error) {
	var r txRow
	err := row.Scan(
		&r.ID, &r.TerminalID, &r.MerchantID,
		&r.State, &r.AmountCents, &r.Currency,
		&r.PanLast4, &r.CardNetwork, &r.EntryMode,
		&r.Stan, &r.AuthCode, &r.RejectionCode, &r.RejectionSource,
		&r.FraudScore, &r.FraudDecision, &r.FraudRulesHit,
		&r.EMVDataB64, &r.ISO8583Raw,
		&r.CreatedAt, &r.AuthorizedAt, &r.RejectedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerrors.NewNotFoundError("Transaction", "")
		}
		return nil, fmt.Errorf("scanTransaction: %w", err)
	}

	// Reconstruir Value Objects
	currency, err := domain.ParseCurrency(r.Currency)
	if err != nil {
		return nil, fmt.Errorf("scanTransaction: parse currency: %w", err)
	}
	amount, err := domain.NewMoney(r.AmountCents, currency)
	if err != nil {
		return nil, fmt.Errorf("scanTransaction: parse money: %w", err)
	}
	txID, _ := domain.ParseTransactionID(r.ID)
	terminalID, _ := domain.ParseTerminalID(r.TerminalID)
	merchantID, _ := domain.ParseMerchantID(r.MerchantID)
	stan, _ := domain.NewSTAN(r.Stan)
	network, _ := domain.ParseCardNetwork(r.CardNetwork)
	pan, _ := domain.NewPAN(r.PanLast4, network)
	entryMode, _ := valueobject.ParseEntryMode(r.EntryMode)

	// Reconstruir FraudDecision si está disponible
	var fraudDecision valueobject.FraudDecision
	if r.FraudScore != nil && r.FraudDecision != nil {
		var rulesHit []string
		if r.FraudRulesHit != nil {
			_ = json.Unmarshal(r.FraudRulesHit, &rulesHit)
		}
		fraudDecision, _ = valueobject.NewFraudDecision(*r.FraudScore, *r.FraudDecision, rulesHit)
	}

	// Reconstruir el aggregate usando el constructor de reconstitución
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:              txID,
		TerminalID:      terminalID,
		MerchantID:      merchantID,
		Amount:          amount,
		STAN:            stan,
		PAN:             pan,
		EntryMode:       entryMode,
		State:           valueobject.TransactionState(r.State),
		FraudDecision:   fraudDecision,
		AuthCode:        r.AuthCode,
		RejectionCode:   r.RejectionCode,
		RejectionSource: r.RejectionSource,
		EMVDataBase64:   r.EMVDataB64,
		ISO8583Raw:      r.ISO8583Raw,
		ReceivedAt:      r.CreatedAt,
		AuthorizedAt:    r.AuthorizedAt,
		RejectedAt:      r.RejectedAt,
	}), nil
}
