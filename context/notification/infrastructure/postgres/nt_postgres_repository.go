// Package postgres contiene el adaptador PostgreSQL del BC Notification.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/entity"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// NotificationRepo implementa repository.NotificationRepository.
type NotificationRepo struct{ pool *pgxpool.Pool }

func NewNotificationRepo(pool *pgxpool.Pool) *NotificationRepo {
	return &NotificationRepo{pool: pool}
}

// Save persiste la Notification y sus DeliveryAttempts nuevos.
func (r *NotificationRepo) Save(ctx context.Context, n *aggregate.Notification) error {
	return pgutil.WithReadCommitted(ctx, r.pool, func(tx pgx.Tx) error {
		receiptJSON, _ := json.Marshal(n.Receipt())

		const upsert = `
			INSERT INTO notification.notifications
				(id, transaction_id, merchant_id, channel, state, receipt,
				 attempt_count, max_attempts, next_retry_at, created_at, dispatched_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (id) DO UPDATE SET
				state         = EXCLUDED.state,
				attempt_count = EXCLUDED.attempt_count,
				next_retry_at = EXCLUDED.next_retry_at,
				dispatched_at = EXCLUDED.dispatched_at
		`
		_, err := tx.Exec(ctx, upsert,
			n.ID(), n.TransactionID().String(), n.MerchantID().String(),
			n.Channel().String(), n.State().String(), receiptJSON,
			n.AttemptCount(), n.MaxAttempts(), n.NextRetryAt(),
			n.CreatedAt(), n.DispatchedAt(),
		)
		if err != nil {
			return fmt.Errorf("NotificationRepo.Save: upsert: %w", err)
		}

		// Insertar solo los DeliveryAttempts nuevos
		const insertAttempt = `
			INSERT INTO notification.delivery_attempts
				(id, notification_id, attempt_number, success, http_status, error_message, attempted_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (id) DO NOTHING
		`
		for _, a := range n.Attempts() {
			if _, err := tx.Exec(ctx, insertAttempt,
				a.ID(), a.NotificationID(), a.AttemptNumber(),
				a.Success(), a.HTTPStatus(), a.ErrorMessage(), a.AttemptedAt(),
			); err != nil {
				return fmt.Errorf("NotificationRepo.Save: insert attempt: %w", err)
			}
		}

		return nil
	})
}

// FindByID recupera una Notification con todos sus DeliveryAttempts.
func (r *NotificationRepo) FindByID(ctx context.Context, id string) (*aggregate.Notification, error) {
	const q = `
		SELECT id, transaction_id, merchant_id, channel, state, receipt,
		       attempt_count, max_attempts, next_retry_at, created_at, dispatched_at
		FROM notification.notifications WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, q, id)
	n, err := scanNotification(row)
	if err != nil {
		return nil, err
	}
	attempts, err := r.loadAttempts(ctx, id)
	if err != nil {
		return nil, err
	}
	return reconstituteNotification(n, attempts), nil
}

// FindByTransactionID recupera todas las notificaciones de una transacción.
func (r *NotificationRepo) FindByTransactionID(
	ctx context.Context,
	txID domain.TransactionID,
) ([]*aggregate.Notification, error) {
	const q = `
		SELECT id, transaction_id, merchant_id, channel, state, receipt,
		       attempt_count, max_attempts, next_retry_at, created_at, dispatched_at
		FROM notification.notifications
		WHERE transaction_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.pool.Query(ctx, q, txID.String())
	if err != nil {
		return nil, fmt.Errorf("FindByTransactionID: %w", err)
	}
	defer rows.Close()

	return r.scanAndLoad(ctx, rows)
}

// FindPendingRetries recupera notificaciones RETRYING cuyo next_retry_at ya pasó.
func (r *NotificationRepo) FindPendingRetries(ctx context.Context, limit int) ([]*aggregate.Notification, error) {
	const q = `
		SELECT id, transaction_id, merchant_id, channel, state, receipt,
		       attempt_count, max_attempts, next_retry_at, created_at, dispatched_at
		FROM notification.notifications
		WHERE state = 'RETRYING'
		  AND next_retry_at <= NOW()
		ORDER BY next_retry_at ASC
		LIMIT $1
	`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("FindPendingRetries: %w", err)
	}
	defer rows.Close()

	return r.scanAndLoad(ctx, rows)
}

// FindDead recupera notificaciones en estado DEAD.
func (r *NotificationRepo) FindDead(ctx context.Context, limit int) ([]*aggregate.Notification, error) {
	const q = `
		SELECT id, transaction_id, merchant_id, channel, state, receipt,
		       attempt_count, max_attempts, next_retry_at, created_at, dispatched_at
		FROM notification.notifications
		WHERE state = 'DEAD'
		ORDER BY created_at DESC
		LIMIT $1
	`
	rows, err := r.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("FindDead: %w", err)
	}
	defer rows.Close()

	return r.scanAndLoad(ctx, rows)
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

type rawNotification struct {
	id, transactionID, merchantID string
	channel, state                string
	receiptJSON                   []byte
	attemptCount, maxAttempts     int
	nextRetryAt                   *time.Time
	createdAt                     time.Time
	dispatchedAt                  *time.Time
}

func scanNotification(row pgx.Row) (*rawNotification, error) {
	var n rawNotification
	err := row.Scan(
		&n.id, &n.transactionID, &n.merchantID,
		&n.channel, &n.state, &n.receiptJSON,
		&n.attemptCount, &n.maxAttempts, &n.nextRetryAt,
		&n.createdAt, &n.dispatchedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pkgerrors.NewNotFoundError("Notification", "")
		}
		return nil, fmt.Errorf("scanNotification: %w", err)
	}
	return &n, nil
}

func (r *NotificationRepo) loadAttempts(ctx context.Context, notifID string) ([]*entity.DeliveryAttempt, error) {
	const q = `
		SELECT id, notification_id, attempt_number, success, http_status, error_message, attempted_at
		FROM notification.delivery_attempts
		WHERE notification_id = $1
		ORDER BY attempt_number ASC
	`
	rows, err := r.pool.Query(ctx, q, notifID)
	if err != nil {
		return nil, fmt.Errorf("loadAttempts: %w", err)
	}
	defer rows.Close()

	var attempts []*entity.DeliveryAttempt
	for rows.Next() {
		var id, notifID, errMsg string
		var attemptNum, httpStatus int
		var success bool
		var attemptedAt time.Time

		if err := rows.Scan(&id, &notifID, &attemptNum, &success, &httpStatus, &errMsg, &attemptedAt); err != nil {
			return nil, fmt.Errorf("loadAttempts: scan: %w", err)
		}
		attempts = append(attempts, entity.ReconstituteDeliveryAttempt(
			id, notifID, attemptNum, success, httpStatus, errMsg, attemptedAt,
		))
	}
	return attempts, nil
}

func (r *NotificationRepo) scanAndLoad(ctx context.Context, rows pgx.Rows) ([]*aggregate.Notification, error) {
	var result []*aggregate.Notification
	for rows.Next() {
		raw, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		attempts, err := r.loadAttempts(ctx, raw.id)
		if err != nil {
			return nil, err
		}
		result = append(result, reconstituteNotification(raw, attempts))
	}
	return result, nil
}

func reconstituteNotification(raw *rawNotification, attempts []*entity.DeliveryAttempt) *aggregate.Notification {
	txID, _ := domain.ParseTransactionID(raw.transactionID)
	mID, _ := domain.ParseMerchantID(raw.merchantID)
	ch, _ := valueobject.ParseNotificationChannel(raw.channel)
	st, _ := valueobject.ParseNotificationState(raw.state)

	var receipt valueobject.ReceiptPayload
	_ = json.Unmarshal(raw.receiptJSON, &receipt)

	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            raw.id,
		TransactionID: txID,
		MerchantID:    mID,
		Channel:       ch,
		State:         st,
		Receipt:       receipt,
		Attempts:      attempts,
		MaxAttempts:   raw.maxAttempts,
		CreatedAt:     raw.createdAt,
		DispatchedAt:  raw.dispatchedAt,
		NextRetryAt:   raw.nextRetryAt,
	})
}
