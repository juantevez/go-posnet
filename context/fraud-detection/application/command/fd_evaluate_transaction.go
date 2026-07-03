// Package command contiene los command handlers del BC Fraud Detection.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/repository"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/service"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// EvaluateTransactionHandler implementa port.FraudService.
// Orquesta: idempotencia → crear FraudCase → ejecutar motor → persistir → publicar.
type EvaluateTransactionHandler struct {
	fraudCaseRepo repository.FraudCaseRepository
	engine        *service.RuleEngine
	publisher     service.EventPublisher
	idempotency   *natsutil.IdempotencyStore
	pool          pgutil.PgxPool
}

// NewEvaluateTransactionHandler construye el handler con sus dependencias.
func NewEvaluateTransactionHandler(
	fraudCaseRepo repository.FraudCaseRepository,
	engine *service.RuleEngine,
	publisher service.EventPublisher,
	idempotency *natsutil.IdempotencyStore,
	pool pgutil.PgxPool,
) *EvaluateTransactionHandler {
	return &EvaluateTransactionHandler{
		fraudCaseRepo: fraudCaseRepo,
		engine:        engine,
		publisher:     publisher,
		idempotency:   idempotency,
		pool:          pool,
	}
}

// EvaluateTransaction es el caso de uso principal del BC Fraud Detection.
//
// Flujo:
//  1. Verificar idempotencia — si el evento ya fue procesado, drop silencioso
//  2. Parsear y validar los Value Objects del comando
//  3. Crear el aggregate FraudCase
//  4. Ejecutar el motor de reglas — evalúa todas las reglas en paralelo
//  5. Persistir el FraudCase con el score calculado + marcar idempotencia (atómico)
//  6. Publicar FraudScoreCalculated a NATS — Authorization BC lo consume
func (h *EvaluateTransactionHandler) EvaluateTransaction(
	ctx context.Context,
	cmd port.EvaluateTransactionCommand,
) error {
	ctx, span := observability.StartSpan(ctx, "command.EvaluateTransaction")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("transaction_id", cmd.TransactionID),
		slog.String("terminal_id", cmd.TerminalID),
		slog.String("event_id", cmd.EventID),
	)

	// ── 1. Parsear Value Objects ─────────────────────────────────────────────
	txID, err := domain.ParseTransactionID(cmd.TransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid transaction_id: " + err.Error())
	}
	terminalID, err := domain.ParseTerminalID(cmd.TerminalID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid terminal_id: " + err.Error())
	}
	merchantID, err := domain.ParseMerchantID(cmd.MerchantID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid merchant_id: " + err.Error())
	}
	occurredAt, err := time.Parse(time.RFC3339, cmd.OccurredAt)
	if err != nil {
		occurredAt = time.Now().UTC() // fallback seguro
	}

	// ── 2. Crear el FraudCase ─────────────────────────────────────────────────
	fc, err := aggregate.NewFraudCase(
		txID, terminalID, merchantID,
		cmd.AmountCents, cmd.Currency,
		cmd.CardNetwork, cmd.EntryMode,
		occurredAt,
	)
	if err != nil {
		return pkgerrors.NewValidationError(err.Error())
	}

	// ── 3. Ejecutar el motor de reglas ───────────────────────────────────────
	if err := h.engine.Evaluate(ctx, fc); err != nil {
		// Si el motor falla completamente, publicar un score neutral (REVIEW)
		// para no bloquear la transacción por un problema de infraestructura.
		log.Error("rule engine failed — using neutral score",
			slog.String("error", err.Error()),
		)
		observability.RecordError(ctx, err)
		// Construir un FraudCase con score neutro para continuar el flujo
		neutralScore, _ := fc.BuildNeutralScore("RULE_ENGINE_FAILURE")
		_ = neutralScore
		// Publicar el score neutro directamente y retornar
		return h.publisher.PublishFraudScoreCalculated(ctx, fc)
	}

	log.Info("fraud evaluation completed",
		slog.Int("score", fc.Score().Score()),
		slog.String("decision", fc.Score().Decision().String()),
		slog.Any("rules_hit", fc.Score().RulesHit()),
	)

	// ── 4. Persistir + marcar idempotencia (atómico) ─────────────────────────
	var published bool
	err = pgutil.WithReadCommitted(ctx, h.pool, func(tx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, tx, cmd.EventID)
		if err != nil {
			return err
		}
		if !inserted {
			log.Info("fraud check event already processed — skipping")
			return nil
		}
		if err := h.fraudCaseRepo.Save(ctx, fc); err != nil {
			return err
		}
		published = true
		return nil
	})
	if err != nil {
		observability.RecordError(ctx, err)
		return fmt.Errorf("EvaluateTransaction: persist: %w", err)
	}
	if !published {
		return nil
	}

	// ── 5. Publicar FraudScoreCalculated a NATS ──────────────────────────────
	if err := h.publisher.PublishFraudScoreCalculated(ctx, fc); err != nil {
		// Loguear pero no fallar — el FraudCase ya está en Postgres.
		// Authorization tiene un mecanismo de bypass por timeout (500ms).
		log.Error("failed to publish fraud score — authorization will apply bypass",
			slog.String("error", err.Error()),
		)
	}

	return nil
}

