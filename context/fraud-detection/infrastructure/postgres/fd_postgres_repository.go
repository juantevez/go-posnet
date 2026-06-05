// Package postgres contiene el adaptador PostgreSQL del BC Fraud Detection.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

// ─── FraudCaseRepo ────────────────────────────────────────────────────────────

// FraudCaseRepo implementa repository.FraudCaseRepository.
type FraudCaseRepo struct{ pool *pgxpool.Pool }

func NewFraudCaseRepo(pool *pgxpool.Pool) *FraudCaseRepo {
	return &FraudCaseRepo{pool: pool}
}

func (r *FraudCaseRepo) Save(ctx context.Context, fc *aggregate.FraudCase) error {
	rulesHit, _ := json.Marshal(fc.Score().RulesHit())

	type evalRow struct {
		RuleID            string `json:"rule_id"`
		Activated         bool   `json:"activated"`
		ScoreContribution int    `json:"score_contribution"`
		Reason            string `json:"reason"`
	}
	evals := make([]evalRow, 0, len(fc.Evaluations()))
	for _, e := range fc.Evaluations() {
		evals = append(evals, evalRow{
			RuleID:            e.RuleID(),
			Activated:         e.Activated(),
			ScoreContribution: e.ScoreContribution(),
			Reason:            e.Reason(),
		})
	}
	evalsJSON, _ := json.Marshal(evals)

	const q = `
		INSERT INTO fraud_detection.fraud_cases (
			id, transaction_id,
			terminal_id, merchant_id, amount_cents, currency, card_network, entry_mode, occurred_at,
			score, decision, rules_hit, evaluations, evaluated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (transaction_id) DO NOTHING
	`
	_, err := r.pool.Exec(ctx, q,
		fc.ID(), fc.TransactionID().String(),
		fc.TerminalID().String(), fc.MerchantID().String(),
		fc.AmountCents(), fc.Currency(), fc.CardNetwork(), fc.EntryMode(), fc.OccurredAt(),
		fc.Score().Score(), fc.Score().Decision().String(),
		rulesHit, evalsJSON,
		fc.EvaluatedAt(),
	)
	if err != nil {
		return fmt.Errorf("FraudCaseRepo.Save: %w", err)
	}
	return nil
}

func (r *FraudCaseRepo) FindByTransactionID(
	ctx context.Context,
	txID domain.TransactionID,
) (*aggregate.FraudCase, error) {
	const q = `
		SELECT id, transaction_id,
		       terminal_id, merchant_id, amount_cents, currency, card_network, entry_mode, occurred_at,
		       score, decision, rules_hit, evaluations, evaluated_at
		FROM fraud_detection.fraud_cases
		WHERE transaction_id = $1
	`
	row := r.pool.QueryRow(ctx, q, txID.String())

	var (
		id, txIDStr                      string
		terminalID, merchantID           string
		amountCents                      int64
		currency, cardNetwork, entryMode string
		occurredAt                       time.Time
		score                            int
		decision                         string
		rulesHitJSON, evaluationsJSON    []byte
		evaluatedAt                      time.Time
	)

	err := row.Scan(
		&id, &txIDStr,
		&terminalID, &merchantID, &amountCents, &currency, &cardNetwork, &entryMode, &occurredAt,
		&score, &decision, &rulesHitJSON, &evaluationsJSON,
		&evaluatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("FraudCaseRepo.FindByTransactionID: %w", err)
	}

	var rulesHit []string
	_ = json.Unmarshal(rulesHitJSON, &rulesHit)

	fraudScore, err := valueobject.NewFraudScore(score, rulesHit)
	if err != nil {
		return nil, fmt.Errorf("FraudCaseRepo: reconstruct score: %w", err)
	}

	type evalRow struct {
		RuleID            string `json:"rule_id"`
		RuleName          string `json:"rule_name"`
		Activated         bool   `json:"activated"`
		ScoreContribution int    `json:"score_contribution"`
		Reason            string `json:"reason"`
	}
	var evalRows []evalRow
	_ = json.Unmarshal(evaluationsJSON, &evalRows)

	evals := make([]valueobject.RuleEvaluation, 0, len(evalRows))
	for _, er := range evalRows {
		eval, err := valueobject.NewRuleEvaluation(er.RuleID, er.RuleName, er.Activated, er.ScoreContribution, er.Reason)
		if err == nil {
			evals = append(evals, eval)
		}
	}

	parsedTxID, _ := domain.ParseTransactionID(txIDStr)
	termID, _ := domain.ParseTerminalID(terminalID)
	merchID, _ := domain.ParseMerchantID(merchantID)

	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            id,
		TransactionID: parsedTxID,
		TerminalID:    termID,
		MerchantID:    merchID,
		AmountCents:   amountCents,
		Currency:      currency,
		CardNetwork:   cardNetwork,
		EntryMode:     entryMode,
		OccurredAt:    occurredAt,
		Score:         fraudScore,
		Evaluations:   evals,
		EvaluatedAt:   &evaluatedAt,
	}), nil
}

// ─── FraudRuleRepo (con cache en memoria) ─────────────────────────────────────

// FraudRuleRepo implementa repository.FraudRuleRepository con cache en memoria.
// Las reglas se recargan desde Postgres cada cacheTTL para permitir
// cambios de configuración sin redespliegue.
type FraudRuleRepo struct {
	pool     *pgxpool.Pool
	cacheTTL time.Duration
	mu       sync.RWMutex
	cached   []*entity.FraudRule
	cachedAt time.Time
}

func NewFraudRuleRepo(pool *pgxpool.Pool, cacheTTL time.Duration) *FraudRuleRepo {
	return &FraudRuleRepo{pool: pool, cacheTTL: cacheTTL}
}

func (r *FraudRuleRepo) FindAllActive(ctx context.Context) ([]*entity.FraudRule, error) {
	r.mu.RLock()
	if r.cached != nil && time.Since(r.cachedAt) < r.cacheTTL {
		rules := r.cached
		r.mu.RUnlock()
		return rules, nil
	}
	r.mu.RUnlock()

	// Cache expirado — recargar desde Postgres
	rules, err := r.loadFromDB(ctx)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cached = rules
	r.cachedAt = time.Now()
	r.mu.Unlock()

	return rules, nil
}

func (r *FraudRuleRepo) loadFromDB(ctx context.Context) ([]*entity.FraudRule, error) {
	const q = `
		SELECT id, name, description, score_weight, threshold_value, is_active, updated_at
		FROM fraud_detection.fraud_rules
		WHERE is_active = TRUE
		ORDER BY score_weight DESC
	`
	rows, err := r.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("FraudRuleRepo.loadFromDB: %w", err)
	}
	defer rows.Close()

	var rules []*entity.FraudRule
	for rows.Next() {
		var id, name, description string
		var scoreWeight int
		var thresholdValue float64
		var isActive bool
		var updatedAt time.Time

		if err := rows.Scan(&id, &name, &description, &scoreWeight, &thresholdValue, &isActive, &updatedAt); err != nil {
			return nil, fmt.Errorf("FraudRuleRepo: scan row: %w", err)
		}
		rules = append(rules, entity.ReconstituteFraudRule(id, name, description, scoreWeight, thresholdValue, isActive, updatedAt))
	}
	return rules, nil
}

func (r *FraudRuleRepo) Save(ctx context.Context, rule *entity.FraudRule) error {
	const q = `
		UPDATE fraud_detection.fraud_rules
		SET score_weight = $1, threshold_value = $2, is_active = $3, updated_at = NOW()
		WHERE id = $4
	`
	tag, err := r.pool.Exec(ctx, q,
		rule.ScoreWeight(), rule.ThresholdValue(), rule.IsActive(), rule.ID(),
	)
	if err != nil {
		return fmt.Errorf("FraudRuleRepo.Save: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pkgerrors.NewNotFoundError("FraudRule", rule.ID())
	}

	// Invalidar cache para forzar recarga
	r.mu.Lock()
	r.cached = nil
	r.mu.Unlock()

	return nil
}

// ─── TransactionHistoryRepo ───────────────────────────────────────────────────

// TransactionHistoryRepo implementa repository.TransactionHistoryRepository.
// Consulta la tabla del BC Authorization (schema authorization) — solo lectura.
// NOTA: en producción considerar una vista materializada o una tabla de resumen
// dedicada para evitar queries costosas en el schema de otro BC.
type TransactionHistoryRepo struct{ pool *pgxpool.Pool }

func NewTransactionHistoryRepo(pool *pgxpool.Pool) *TransactionHistoryRepo {
	return &TransactionHistoryRepo{pool: pool}
}

func (r *TransactionHistoryRepo) CountByTerminalLastHour(
	ctx context.Context,
	terminalID domain.TerminalID,
) (int, error) {
	const q = `
		SELECT COUNT(*) FROM authorization.transactions
		WHERE terminal_id = $1
		  AND created_at >= NOW() - INTERVAL '1 hour'
	`
	var count int
	err := r.pool.QueryRow(ctx, q, terminalID.String()).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("TransactionHistoryRepo.CountByTerminalLastHour: %w", err)
	}
	return count, nil
}

func (r *TransactionHistoryRepo) AverageAmountByMerchant(
	ctx context.Context,
	merchantID domain.MerchantID,
) (int64, error) {
	const q = `
		SELECT COALESCE(AVG(amount_cents)::BIGINT, 0)
		FROM authorization.transactions
		WHERE merchant_id = $1
		  AND created_at >= NOW() - INTERVAL '30 days'
		  AND state = 'APPROVED'
	`
	var avg int64
	err := r.pool.QueryRow(ctx, q, merchantID.String()).Scan(&avg)
	if err != nil {
		return 0, fmt.Errorf("TransactionHistoryRepo.AverageAmountByMerchant: %w", err)
	}
	return avg, nil
}

func (r *TransactionHistoryRepo) CountRecentRejectionsByTerminal(
	ctx context.Context,
	terminalID domain.TerminalID,
	lastMinutes int,
) (int, error) {
	const q = `
		SELECT COUNT(*) FROM authorization.transactions
		WHERE terminal_id = $1
		  AND state = 'REJECTED'
		  AND created_at >= NOW() - ($2 || ' minutes')::INTERVAL
	`
	var count int
	err := r.pool.QueryRow(ctx, q, terminalID.String(), lastMinutes).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("TransactionHistoryRepo.CountRecentRejectionsByTerminal: %w", err)
	}
	return count, nil
}

func (r *TransactionHistoryRepo) CountSameAmountAttempts(
	ctx context.Context,
	terminalID domain.TerminalID,
	amountCents int64,
	lastMinutes int,
) (int, error) {
	const q = `
		SELECT COUNT(*) FROM authorization.transactions
		WHERE terminal_id = $1
		  AND amount_cents = $2
		  AND created_at >= NOW() - ($3 || ' minutes')::INTERVAL
	`
	var count int
	err := r.pool.QueryRow(ctx, q, terminalID.String(), amountCents, lastMinutes).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("TransactionHistoryRepo.CountSameAmountAttempts: %w", err)
	}
	return count, nil
}
