package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/port"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func TestEvaluateTransaction_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*port.EvaluateTransactionCommand)
		wantErr string
	}{
		{"invalid transaction id", func(c *port.EvaluateTransactionCommand) { c.TransactionID = "not-a-uuid" }, "invalid transaction_id"},
		{"invalid terminal id", func(c *port.EvaluateTransactionCommand) { c.TerminalID = "not-a-uuid" }, "invalid terminal_id"},
		{"invalid merchant id", func(c *port.EvaluateTransactionCommand) { c.MerchantID = "not-a-uuid" }, "invalid merchant_id"},
		{"non-positive amount", func(c *port.EvaluateTransactionCommand) { c.AmountCents = 0 }, "amount_cents must be positive"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validEvaluateCmd(t)
			tc.mutate(&cmd)

			engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
			h := command.NewEvaluateTransactionHandler(
				&fakeFraudCaseRepo{}, engine, &fakePublisher{},
				natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
			)

			err := h.EvaluateTransaction(context.Background(), cmd)
			var ve *pkgerrors.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestEvaluateTransaction_OccurredAtFallback(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeFraudCaseRepo{}
	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		repo, engine, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validEvaluateCmd(t)
	cmd.OccurredAt = "not-a-date"

	before := time.Now().UTC()
	if err := h.EvaluateTransaction(context.Background(), cmd); err != nil {
		t.Fatalf("EvaluateTransaction() error = %v", err)
	}
	after := time.Now().UTC()

	if len(repo.savedCases) != 1 {
		t.Fatalf("saved cases = %d, want 1", len(repo.savedCases))
	}
	occurredAt := repo.savedCases[0].OccurredAt()
	if occurredAt.Before(before) || occurredAt.After(after) {
		t.Errorf("OccurredAt() = %v, want between %v and %v (fallback a time.Now())", occurredAt, before, after)
	}
}

func TestEvaluateTransaction_EngineFailure_PublishesNeutralScore(t *testing.T) {
	// ruleRepo sin reglas activas → RuleEngine.Evaluate() falla con
	// "no active rules found" → el handler debe recurrir a un score neutral.
	engine := newEngine(nil, 0)
	repo := &fakeFraudCaseRepo{}
	pub := &fakePublisher{}
	h := command.NewEvaluateTransactionHandler(
		repo, engine, pub,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	)

	if err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t)); err != nil {
		t.Fatalf("EvaluateTransaction() error = %v", err)
	}
	if pub.publishCalls != 1 {
		t.Fatalf("PublishFraudScoreCalculated calls = %d, want 1", pub.publishCalls)
	}
	if pub.lastFraudCase.Score().Score() != 50 {
		t.Errorf("published Score().Score() = %d, want 50 (neutral)", pub.lastFraudCase.Score().Score())
	}
	if pub.lastFraudCase.Score().Decision() != valueobject.DecisionReview {
		t.Errorf("published Score().Decision() = %v, want %v", pub.lastFraudCase.Score().Decision(), valueobject.DecisionReview)
	}
	// El flujo de engine failure no persiste en Postgres — no debe tocar el repo.
	if len(repo.savedCases) != 0 {
		t.Errorf("saved cases = %d, want 0 (engine failure no persiste)", len(repo.savedCases))
	}
}

func TestEvaluateTransaction_EngineFailureAndPublishError(t *testing.T) {
	engine := newEngine(nil, 0)
	pub := &fakePublisher{publishErr: errors.New("nats unavailable")}
	h := command.NewEvaluateTransactionHandler(
		&fakeFraudCaseRepo{}, engine, pub,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	)

	err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t))
	if err == nil || !strings.Contains(err.Error(), "nats unavailable") {
		t.Fatalf("error = %v, want it to contain %q", err, "nats unavailable")
	}
}

func TestEvaluateTransaction_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeFraudCaseRepo{}
	pub := &fakePublisher{}
	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		repo, engine, pub,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t)); err != nil {
		t.Fatalf("EvaluateTransaction() error = %v", err)
	}
	if len(repo.savedCases) != 1 {
		t.Fatalf("saved cases = %d, want 1", len(repo.savedCases))
	}
	if repo.savedCases[0].Score().Score() != 20 {
		t.Errorf("saved Score().Score() = %d, want 20 (RULE-005 activada)", repo.savedCases[0].Score().Score())
	}
	if pub.publishCalls != 1 {
		t.Errorf("PublishFraudScoreCalculated calls = %d, want 1", pub.publishCalls)
	}
}

func TestEvaluateTransaction_DuplicateEventSkipsSaveAndPublish(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0) // 0 filas afectadas → event_id ya existía

	repo := &fakeFraudCaseRepo{}
	pub := &fakePublisher{}
	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		repo, engine, pub,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t)); err != nil {
		t.Fatalf("EvaluateTransaction() error = %v", err)
	}
	if len(repo.savedCases) != 0 {
		t.Errorf("saved cases = %d, want 0 (duplicado no debe persistirse)", len(repo.savedCases))
	}
	if pub.publishCalls != 0 {
		t.Errorf("PublishFraudScoreCalculated calls = %d, want 0 (duplicado no debe republicarse)", pub.publishCalls)
	}
}

func TestEvaluateTransaction_ClaimExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		&fakeFraudCaseRepo{}, engine, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t))
	if err == nil || !strings.Contains(err.Error(), "EvaluateTransaction: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "EvaluateTransaction: persist")
	}
}

func TestEvaluateTransaction_SaveError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	repo := &fakeFraudCaseRepo{saveErr: errors.New("db unreachable")}
	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		repo, engine, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t))
	if err == nil || !strings.Contains(err.Error(), "EvaluateTransaction: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "EvaluateTransaction: persist")
	}
}

func TestEvaluateTransaction_PublishErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeFraudCaseRepo{}
	pub := &fakePublisher{publishErr: errors.New("nats unavailable")}
	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		repo, engine, pub,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t)); err != nil {
		t.Fatalf("EvaluateTransaction() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedCases) != 1 {
		t.Errorf("saved cases = %d, want 1", len(repo.savedCases))
	}
}

func TestEvaluateTransaction_BeginTxError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	engine := newEngine([]*entity.FraudRule{mustFraudRule(t, "RULE-005", 20)}, 0)
	h := command.NewEvaluateTransactionHandler(
		&fakeFraudCaseRepo{}, engine, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.EvaluateTransaction(context.Background(), validEvaluateCmd(t))
	if err == nil || !strings.Contains(err.Error(), "EvaluateTransaction: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "EvaluateTransaction: persist")
	}
}
