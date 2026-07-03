package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/juantevez/go-posnet/context/authorization/application/command"
	"github.com/juantevez/go-posnet/context/authorization/domain/service"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func TestApplyFraudScore_DuplicateEventIsSkipped(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0)

	repo := &fakeRepo{}
	acquirer := &fakeAcquirer{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, domain.NewTransactionID()))
	if err != nil {
		t.Fatalf("ApplyFraudScore() error = %v", err)
	}
	if acquirer.authorizeCalls != 0 {
		t.Errorf("Authorize calls = %d, want 0", acquirer.authorizeCalls)
	}
	if len(repo.savedTxs) != 0 {
		t.Errorf("saved txs = %d, want 0", len(repo.savedTxs))
	}
}

func TestApplyFraudScore_ClaimError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	h := command.NewAuthorizationHandler(
		&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, domain.NewTransactionID()))
	if err == nil || !strings.Contains(err.Error(), "claim event") {
		t.Fatalf("error = %v, want it to contain %q", err, "claim event")
	}
}

func TestApplyFraudScore_InvalidTransactionID(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	h := command.NewAuthorizationHandler(
		&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validFraudScoreCmd(t, domain.NewTransactionID())
	cmd.TransactionID = "not-a-uuid"

	err := h.ApplyFraudScore(context.Background(), cmd)
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestApplyFraudScore_FindByIDError(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findErr: errors.New("not found")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, domain.NewTransactionID()))
	if err == nil || !strings.Contains(err.Error(), "find transaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "find transaction")
	}
}

func TestApplyFraudScore_InvalidFraudDecision(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validFraudScoreCmd(t, repo.findResult.ID())
	cmd.Score = 999 // fuera de rango [0,100]

	err := h.ApplyFraudScore(context.Background(), cmd)
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestApplyFraudScore_ApplyDecisionErrorOnWrongState(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	// tx en RECEIVED (nunca pasó por StartFraudCheck) → ApplyFraudDecision debe fallar.
	repo := &fakeRepo{findResult: newValidTransaction(t)}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID()))
	if err == nil || !strings.Contains(err.Error(), "apply decision") {
		t.Fatalf("error = %v, want it to contain %q", err, "apply decision")
	}
}

func TestApplyFraudScore_RejectByFraud(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validFraudScoreCmd(t, repo.findResult.ID())
	cmd.Decision = valueobject.FraudDecisionReject
	cmd.Score = 90

	if err := h.ApplyFraudScore(context.Background(), cmd); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateRejected {
		t.Fatalf("saved tx = %+v, want a single tx in state REJECTED", repo.savedTxs)
	}
	if publisher.rejectedCalls != 1 {
		t.Errorf("PublishRejected calls = %d, want 1", publisher.rejectedCalls)
	}
}

func TestApplyFraudScore_RejectByFraud_SaveError(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t), saveErr: errors.New("db unreachable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validFraudScoreCmd(t, repo.findResult.ID())
	cmd.Decision = valueobject.FraudDecisionReject
	cmd.Score = 90

	err := h.ApplyFraudScore(context.Background(), cmd)
	if err == nil || !strings.Contains(err.Error(), "persistAndPublishRejection") {
		t.Fatalf("error = %v, want it to contain %q", err, "persistAndPublishRejection")
	}
}

func TestApplyFraudScore_RejectByFraud_PublishErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	publisher := &fakePublisher{rejectedErr: errors.New("nats unavailable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validFraudScoreCmd(t, repo.findResult.ID())
	cmd.Decision = valueobject.FraudDecisionReject
	cmd.Score = 90

	if err := h.ApplyFraudScore(context.Background(), cmd); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateRejected {
		t.Fatalf("saved tx = %+v, want a single tx in state REJECTED", repo.savedTxs)
	}
}

func TestApplyFraudScore_ApproveThenAcquirerApproves(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED,
		AuthCode:     "AB1234",
	}}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateApproved {
		t.Fatalf("saved tx = %+v, want a single tx in state APPROVED", repo.savedTxs)
	}
	if ac := repo.savedTxs[0].AuthCode(); ac == nil || ac.String() != "AB1234" {
		t.Errorf("AuthCode() = %v, want AB1234", ac)
	}
	if publisher.approvedCalls != 1 {
		t.Errorf("PublishApproved calls = %d, want 1", publisher.approvedCalls)
	}
}

func TestApplyFraudScore_ApproveThenPublishApprovedErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED,
		AuthCode:     "AB1234",
	}}
	publisher := &fakePublisher{approvedErr: errors.New("nats unavailable")}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateApproved {
		t.Fatalf("saved tx = %+v, want a single tx in state APPROVED", repo.savedTxs)
	}
}

func TestApplyFraudScore_AcquirerErrorMarksIndeterminate(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeErr: errors.New("acquirer timeout")}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID()))
	if err != nil {
		t.Fatalf("ApplyFraudScore() error = %v, want nil (an acquirer error becomes INDETERMINATE, not a saga failure)", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateIndeterminate {
		t.Fatalf("saved tx = %+v, want a single tx in state INDETERMINATE", repo.savedTxs)
	}
}

func TestApplyFraudScore_AcquirerRejectsWithISOCode(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{ResponseCode: valueobject.ISO_DO_NOT_HONOR}}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v", err)
	}
	rc := repo.savedTxs[0].RejectionCode()
	if rc == nil || rc.Code() != valueobject.ISO_DO_NOT_HONOR {
		t.Fatalf("RejectionCode() = %v, want code %q", rc, valueobject.ISO_DO_NOT_HONOR)
	}
	if publisher.rejectedCalls != 1 {
		t.Errorf("PublishRejected calls = %d, want 1", publisher.rejectedCalls)
	}
}

func TestApplyFraudScore_AcquirerRejectsThenPublishErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{ResponseCode: valueobject.ISO_DO_NOT_HONOR}}
	publisher := &fakePublisher{rejectedErr: errors.New("nats unavailable")}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateRejected {
		t.Fatalf("saved tx = %+v, want a single tx in state REJECTED", repo.savedTxs)
	}
}

func TestApplyFraudScore_AcquirerRejectsWithUnknownResponseCode(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	// ResponseCode vacío → AcquirerResponse.ToRejectionCode() falla →
	// callAcquirer cae al fallback NewRejectionFromValidation("UNKNOWN_RESPONSE").
	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{ResponseCode: ""}}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ApplyFraudScore() error = %v", err)
	}
	rc := repo.savedTxs[0].RejectionCode()
	if rc == nil || rc.Code() != "UNKNOWN_RESPONSE" || rc.Source() != valueobject.SourceValidation {
		t.Fatalf("RejectionCode() = %v, want UNKNOWN_RESPONSE/VALIDATION", rc)
	}
}

func TestApplyFraudScore_ApproveWithInvalidAuthCode(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t)}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED,
		AuthCode:     "not-6-chars",
	}}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID()))
	if err == nil || !strings.Contains(err.Error(), "invalid auth code") {
		t.Fatalf("error = %v, want it to contain %q", err, "invalid auth code")
	}
	if len(repo.savedTxs) != 0 {
		t.Errorf("saved txs = %d, want 0 (should fail before persisting)", len(repo.savedTxs))
	}
}

func TestApplyFraudScore_SaveErrorAfterAcquirerApproval(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newFraudCheckingTransaction(t), saveErr: errors.New("db unreachable")}
	acquirer := &fakeAcquirer{authorizeResp: service.AcquirerResponse{
		ResponseCode: valueobject.ISO_APPROVED,
		AuthCode:     "AB1234",
	}}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ApplyFraudScore(context.Background(), validFraudScoreCmd(t, repo.findResult.ID()))
	if err == nil || !strings.Contains(err.Error(), "persist result") {
		t.Fatalf("error = %v, want it to contain %q", err, "persist result")
	}
}
