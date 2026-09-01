// Package command contiene los command handlers del BC Authorization.
// Implementan los puertos de entrada definidos en application/port.
package command

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/context/authorization/domain/aggregate"
	"github.com/juantevez/go-posnet/context/authorization/domain/repository"
	"github.com/juantevez/go-posnet/context/authorization/domain/service"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

/*const (
	fraudCheckTimeout = 500 * time.Millisecond // Timeout para esperar el score de fraude
)*/

// AuthorizationHandler implementa port.AuthorizationService.
// Orquesta la Saga de autorización completa.
type AuthorizationHandler struct {
	repo         repository.TransactionRepository
	acquirer     service.AcquirerGateway
	publisher    service.EventPublisher
	idempotency  *natsutil.IdempotencyStore
	pool         pgutil.PgxPool
	metrics      *Metrics                         // opcional; nil = sin instrumentación (tests)
	blockedCards repository.BlockedCardRepository // opcional; nil = blocklist deshabilitada
}

// SetBlockedCards inyecta la blocklist de tarjetas. Se llama desde el wiring
// (cmd/authorization). Si no se inyecta, la blocklist queda deshabilitada y
// el wiring lo advierte: sigue el mismo patrón que SetMetrics para no romper
// las firmas que construyen el handler en los tests.
func (h *AuthorizationHandler) SetBlockedCards(r repository.BlockedCardRepository) {
	h.blockedCards = r
}

// SetMetrics inyecta los instrumentos de negocio. Se llama desde el wiring
// (cmd/authorization) tras InitMeter. Mantener la firma de NewAuthorizationHandler
// intacta evita tocar los tests que construyen el handler.
func (h *AuthorizationHandler) SetMetrics(m *Metrics) {
	h.metrics = m
}

// NewAuthorizationHandler construye el handler con todas sus dependencias.
func NewAuthorizationHandler(
	repo repository.TransactionRepository,
	acquirer service.AcquirerGateway,
	publisher service.EventPublisher,
	idempotency *natsutil.IdempotencyStore,
	pool pgutil.PgxPool,
) *AuthorizationHandler {
	return &AuthorizationHandler{
		repo:        repo,
		acquirer:    acquirer,
		publisher:   publisher,
		idempotency: idempotency,
		pool:        pool,
	}
}

// AuthorizeTransaction es el punto de entrada de la Saga de autorización.
// Implementa port.AuthorizationService.
//
// Pasos:
//  1. Verificar idempotencia (event_id ya procesado → drop)
//  2. Parsear y validar el comando
//  3. Crear el aggregate Transaction
//  4. Persistir en Postgres (estado RECEIVED)
//  5. Solicitar fraud check → publicar FraudCheckRequested
//  6. El score llega vía ApplyFraudScore (evento NATS separado)
func (h *AuthorizationHandler) AuthorizeTransaction(ctx context.Context, cmd port.AuthorizeTransactionCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.AuthorizeTransaction")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("transaction_id", cmd.TransactionID),
		slog.String("terminal_id", cmd.TerminalID),
		slog.String("event_id", cmd.EventID),
	)

	// ── Paso 1: Parsear y validar los Value Objects ──────────────────────────
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
	currency, err := domain.ParseCurrency(cmd.Currency)
	if err != nil {
		return pkgerrors.NewValidationError("invalid currency: " + err.Error())
	}
	amount, err := domain.NewMoney(cmd.AmountCents, currency)
	if err != nil {
		return pkgerrors.NewValidationError("invalid amount: " + err.Error())
	}
	stan, err := domain.NewSTAN(cmd.STAN)
	if err != nil {
		return pkgerrors.NewValidationError("invalid stan: " + err.Error())
	}
	network, err := domain.ParseCardNetwork(cmd.CardNetwork)
	if err != nil {
		return pkgerrors.NewValidationError("invalid card_network: " + err.Error())
	}
	pan, err := domain.NewPAN(cmd.CardLast4, network)
	if err != nil {
		return pkgerrors.NewValidationError("invalid pan: " + err.Error())
	}
	entryMode, err := valueobject.ParseEntryMode(cmd.EntryMode)
	if err != nil {
		return pkgerrors.NewValidationError("invalid entry_mode: " + err.Error())
	}
	cardToken, err := domain.ParseOptionalCardToken(cmd.CardToken)
	if err != nil {
		return pkgerrors.NewValidationError("invalid card_token: " + err.Error())
	}

	// ── Paso 3: Crear el aggregate ───────────────────────────────────────────
	tx, err := aggregate.NewTransaction(
		txID, terminalID, merchantID,
		amount, stan, pan, entryMode,
		cardToken, cmd.EMVDataBase64, cmd.ISO8583Raw,
	)
	if err != nil {
		return pkgerrors.NewValidationError(err.Error())
	}

	// ── Blocklist: la tarjeta fue reportada como robada/perdida ──────────────
	// Se corta acá, antes del fraud check y del adquirente: la tarjeta ya está
	// bloqueada, no hay nada que consultar. Si la consulta a la blocklist
	// falla se deja seguir la Saga en vez de rechazar: un error de la base no
	// debe convertirse en un rechazo con orden de retener el plástico.
	blocked, err := h.isCardBlocked(ctx, cardToken)
	if err != nil {
		log.Error("blocklist lookup failed — continuing without the check",
			slog.String("error", err.Error()))
	}
	if blocked {
		return h.rejectBlockedCard(ctx, tx, cmd.EventID, log)
	}

	if err := tx.StartFraudCheck(); err != nil {
		return fmt.Errorf("AuthorizeTransaction: start fraud check: %w", err)
	}

	// ── Paso 4: Persistir + marcar idempotencia en una sola transacción ──────
	var published bool
	err = pgutil.WithReadCommitted(ctx, h.pool, func(dbTx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, dbTx, cmd.EventID)
		if err != nil {
			return err
		}
		if !inserted {
			log.Info("event already processed — skipping")
			return nil
		}
		if err := h.repo.Save(ctx, tx); err != nil {
			return fmt.Errorf("save transaction: %w", err)
		}
		published = true
		return nil
	})
	if err != nil {
		observability.RecordError(ctx, err)
		return fmt.Errorf("AuthorizeTransaction: persist: %w", err)
	}
	if !published {
		return nil
	}
	h.metrics.RecordReceived(ctx)

	// ── Paso 5: Publicar FraudCheckRequested ─────────────────────────────────
	if err := h.publisher.PublishFraudCheckRequested(ctx, tx); err != nil {
		// El fraud check es asíncrono — si falla la publicación, loguear y continuar.
		// La Saga tiene un mecanismo de bypass por timeout en ApplyFraudScore.
		log.Error("failed to publish fraud check request — fraud bypass will apply",
			slog.String("error", err.Error()))
	}

	log.Info("transaction received and fraud check requested",
		slog.String("state", tx.State().String()),
		slog.Int64("amount_cents", cmd.AmountCents),
	)
	return nil
}

// ApplyFraudScore procesa el resultado del motor antifraude y continúa la Saga.
// Es llamado por el subscriber NATS al recibir FraudScoreCalculated.
func (h *AuthorizationHandler) ApplyFraudScore(ctx context.Context, cmd port.ApplyFraudScoreCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.ApplyFraudScore")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("transaction_id", cmd.TransactionID),
		slog.String("event_id", cmd.EventID),
		slog.Int("fraud_score", cmd.Score),
		slog.String("fraud_decision", cmd.Decision),
	)

	var claimed bool
	if err := pgutil.WithReadCommitted(ctx, h.pool, func(dbTx pgx.Tx) error {
		var e error
		claimed, e = h.idempotency.TryMarkAsProcessed(ctx, dbTx, cmd.EventID)
		return e
	}); err != nil {
		return fmt.Errorf("ApplyFraudScore: claim event: %w", err)
	}
	if !claimed {
		log.Info("fraud score event already processed — skipping")
		return nil
	}

	txID, err := domain.ParseTransactionID(cmd.TransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid transaction_id: " + err.Error())
	}

	tx, err := h.repo.FindByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("ApplyFraudScore: find transaction: %w", err)
	}

	fraudDecision, err := valueobject.NewFraudDecision(cmd.Score, cmd.Decision, cmd.RulesHit)
	if err != nil {
		return pkgerrors.NewValidationError("invalid fraud decision: " + err.Error())
	}

	if err := tx.ApplyFraudDecision(fraudDecision); err != nil {
		return fmt.Errorf("ApplyFraudScore: apply decision: %w", err)
	}

	// Si el fraud rechazó → publicar rechazo y terminar
	if tx.State() == valueobject.StateRejected {
		return h.persistAndPublishRejection(ctx, tx, log)
	}

	// Fraud aprobó → llamar al adquirente
	return h.callAcquirer(ctx, tx, log)
}

// callAcquirer envía la transacción al host adquirente y procesa la respuesta.
// El evento ya fue reclamado por TryMarkAsProcessed en ApplyFraudScore, por lo
// que cualquier error del adquirente se convierte en INDETERMINATE: NATS no
// redeliverirá y la conciliación nocturna resuelve el resultado.
func (h *AuthorizationHandler) callAcquirer(
	ctx context.Context,
	tx *aggregate.Transaction,
	log *slog.Logger,
) error {
	ctx, span := observability.StartSpan(ctx, "command.callAcquirer")
	defer span.End()

	acquirerStart := time.Now()
	response, err := h.acquirer.Authorize(ctx, tx)
	if err != nil {
		h.metrics.RecordAcquirerDuration(ctx, time.Since(acquirerStart), "error")
		h.metrics.RecordAcquirerError(ctx)
		log.Warn("acquirer error — marking transaction as indeterminate",
			slog.String("error", err.Error()))
		if markErr := tx.MarkIndeterminate(); markErr != nil {
			return fmt.Errorf("callAcquirer: mark indeterminate: %w", markErr)
		}
		_ = h.repo.Save(ctx, tx)
		return nil
	}
	if response.IsApproved() {
		h.metrics.RecordAcquirerDuration(ctx, time.Since(acquirerStart), "approved")
	} else {
		h.metrics.RecordAcquirerDuration(ctx, time.Since(acquirerStart), "declined")
	}

	if response.IsApproved() {
		authCode, err := domain.NewAuthCode(response.AuthCode)
		if err != nil {
			return fmt.Errorf("callAcquirer: invalid auth code %q: %w", response.AuthCode, err)
		}
		if err := tx.Approve(authCode); err != nil {
			return fmt.Errorf("callAcquirer: approve: %w", err)
		}
	} else {
		rc, err := response.ToRejectionCode()
		if err != nil {
			rc = valueobject.NewRejectionFromValidation("UNKNOWN_RESPONSE")
		}
		if err := tx.Reject(rc); err != nil {
			return fmt.Errorf("callAcquirer: reject: %w", err)
		}
		// El emisor ordenó retener la tarjeta (ISO 04/41/43): además de
		// rechazar esta transacción, la tarjeta entra a la blocklist para que
		// el próximo intento se corte sin llegar al adquirente.
		h.blockCard(ctx, tx, rc, log)
	}

	if err = h.repo.Save(ctx, tx); err != nil {
		observability.RecordError(ctx, err)
		return fmt.Errorf("callAcquirer: persist result: %w", err)
	}

	// Publicar resultado a NATS (fuera de la transacción Postgres)
	if tx.State() == valueobject.StateApproved {
		h.metrics.RecordApproved(ctx)
		h.metrics.RecordSagaDuration(ctx, tx.ReceivedAt(), "approved")
		if err := h.publisher.PublishApproved(ctx, tx); err != nil {
			// Loguear pero no fallar — la transacción ya está en Postgres.
			// El proceso de reconciliación puede republicar si es necesario.
			log.Error("failed to publish approval event", slog.String("error", err.Error()))
		}
		log.Info("transaction approved", slog.String("auth_code", tx.AuthCode().String()))
	} else {
		rc := tx.RejectionCode()
		h.metrics.RecordRejected(ctx, rc.Code(), string(rc.Source()))
		h.metrics.RecordSagaDuration(ctx, tx.ReceivedAt(), "rejected")
		if err := h.publisher.PublishRejected(ctx, tx); err != nil {
			log.Error("failed to publish rejection event", slog.String("error", err.Error()))
		}
		log.Info("transaction rejected",
			slog.String("rejection_code", rc.Code()),
			slog.String("rejection_source", string(rc.Source())),
		)
	}

	return nil
}

// isCardBlocked consulta la blocklist. Con la blocklist deshabilitada o sin
// token la tarjeta nunca está bloqueada — no se puede inferir la identidad de
// una tarjeta a partir de los últimos 4 dígitos.
func (h *AuthorizationHandler) isCardBlocked(ctx context.Context, token domain.CardToken) (bool, error) {
	if h.blockedCards == nil || token.IsZero() {
		return false, nil
	}
	return h.blockedCards.IsBlocked(ctx, token)
}

// blockCard agrega la tarjeta a la blocklist cuando el rechazo trae orden de
// retención. No propaga el error: el rechazo de esta transacción ya es
// correcto y no debe revertirse porque la escritura en la blocklist falle —
// se loguea en ERROR para que quede visible que el bloqueo no persistió.
func (h *AuthorizationHandler) blockCard(
	ctx context.Context,
	tx *aggregate.Transaction,
	rc valueobject.RejectionCode,
	log *slog.Logger,
) {
	if !rc.RequiresCardCapture() {
		return
	}
	if tx.CardToken().IsZero() {
		log.Warn("issuer ordered card capture but no card token was provided — card cannot be blocked",
			slog.String("rejection_code", rc.Code()))
		return
	}
	if h.blockedCards == nil {
		log.Warn("issuer ordered card capture but the blocklist is disabled — card not blocked",
			slog.String("rejection_code", rc.Code()))
		return
	}
	if err := h.blockedCards.Block(ctx, tx.CardToken(), rc.Code(), tx.ID()); err != nil {
		observability.RecordError(ctx, err)
		log.Error("failed to add card to blocklist",
			slog.String("rejection_code", rc.Code()),
			slog.String("error", err.Error()))
		return
	}
	log.Warn("card added to blocklist by issuer capture order",
		slog.String("rejection_code", rc.Code()),
		slog.String("card_token", tx.CardToken().String()))
}

// rejectBlockedCard rechaza una transacción cuya tarjeta ya está bloqueada,
// reclamando el evento en la misma transacción de base que el rechazo para
// que un redelivery de NATS no vuelva a publicarlo.
func (h *AuthorizationHandler) rejectBlockedCard(
	ctx context.Context,
	tx *aggregate.Transaction,
	eventID string,
	log *slog.Logger,
) error {
	rc := valueobject.NewRejectionFromBlocklist()
	if err := tx.Reject(rc); err != nil {
		return fmt.Errorf("rejectBlockedCard: reject: %w", err)
	}

	var claimed bool
	if err := pgutil.WithReadCommitted(ctx, h.pool, func(dbTx pgx.Tx) error {
		inserted, err := h.idempotency.TryMarkAsProcessed(ctx, dbTx, eventID)
		if err != nil {
			return err
		}
		if !inserted {
			return nil
		}
		claimed = true
		return h.repo.Save(ctx, tx)
	}); err != nil {
		observability.RecordError(ctx, err)
		return fmt.Errorf("rejectBlockedCard: persist: %w", err)
	}
	if !claimed {
		log.Info("event already processed — skipping")
		return nil
	}

	h.metrics.RecordReceived(ctx)
	log.Warn("transaction rejected — card is on the blocklist",
		slog.String("rejection_code", rc.Code()),
		slog.String("card_token", tx.CardToken().String()))
	h.publishRejection(ctx, tx, log)
	return nil
}

// persistAndPublishRejection persiste el rechazo y publica el evento.
// El evento ya fue reclamado por TryMarkAsProcessed en ApplyFraudScore.
func (h *AuthorizationHandler) persistAndPublishRejection(
	ctx context.Context,
	tx *aggregate.Transaction,
	log *slog.Logger,
) error {
	if err := h.repo.Save(ctx, tx); err != nil {
		return fmt.Errorf("persistAndPublishRejection: %w", err)
	}
	h.publishRejection(ctx, tx, log)
	return nil
}

// publishRejection registra las métricas del rechazo y lo publica a NATS.
// Separado de la persistencia porque los rechazos que ya se guardaron dentro
// de una transacción de base solo necesitan esta mitad.
func (h *AuthorizationHandler) publishRejection(
	ctx context.Context,
	tx *aggregate.Transaction,
	log *slog.Logger,
) {
	rc := tx.RejectionCode()
	h.metrics.RecordRejected(ctx, rc.Code(), string(rc.Source()))
	h.metrics.RecordSagaDuration(ctx, tx.ReceivedAt(), "rejected")
	if err := h.publisher.PublishRejected(ctx, tx); err != nil {
		log.Error("failed to publish rejection", slog.String("error", err.Error()))
	}
}

// ProcessReversal procesa la anulación de una transacción aprobada.
func (h *AuthorizationHandler) ProcessReversal(ctx context.Context, cmd port.ProcessReversalCommand) error {
	ctx, span := observability.StartSpan(ctx, "command.ProcessReversal")
	defer span.End()

	log := observability.FromContext(ctx).With(
		slog.String("original_tx_id", cmd.OriginalTransactionID),
		slog.String("event_id", cmd.EventID),
	)

	var claimed bool
	if err := pgutil.WithReadCommitted(ctx, h.pool, func(dbTx pgx.Tx) error {
		var e error
		claimed, e = h.idempotency.TryMarkAsProcessed(ctx, dbTx, cmd.EventID)
		return e
	}); err != nil {
		return fmt.Errorf("ProcessReversal: claim event: %w", err)
	}
	if !claimed {
		log.Info("reversal event already processed — skipping")
		return nil
	}

	txID, err := domain.ParseTransactionID(cmd.OriginalTransactionID)
	if err != nil {
		return pkgerrors.NewValidationError("invalid original_transaction_id")
	}

	tx, err := h.repo.FindByID(ctx, txID)
	if err != nil {
		return fmt.Errorf("ProcessReversal: find transaction: %w", err)
	}

	// Enviar reversal al adquirente
	if err := h.acquirer.Reverse(ctx, tx); err != nil {
		// El evento ya fue reclamado — NATS no va a redeliveriar.
		// Loguear y dejar para conciliación manual.
		log.Error("acquirer reversal failed — leaving for reconciliation",
			slog.String("error", err.Error()))
		return nil
	}

	if err := tx.Reverse(); err != nil {
		return fmt.Errorf("ProcessReversal: reverse aggregate: %w", err)
	}

	if err := h.repo.Save(ctx, tx); err != nil {
		return fmt.Errorf("ProcessReversal: persist: %w", err)
	}

	if err := h.publisher.PublishReversalCompleted(ctx, txID, tx); err != nil {
		log.Error("failed to publish reversal completed", slog.String("error", err.Error()))
	}

	log.Info("reversal completed successfully")
	return nil
}
