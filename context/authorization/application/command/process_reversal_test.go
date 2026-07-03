package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/juantevez/go-posnet/context/authorization/application/command"
	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func TestProcessReversal_DuplicateEventIsSkipped(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0)

	repo := &fakeRepo{}
	acquirer := &fakeAcquirer{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, domain.NewTransactionID()))
	if err != nil {
		t.Fatalf("ProcessReversal() error = %v", err)
	}
	if acquirer.reverseCalls != 0 {
		t.Errorf("Reverse calls = %d, want 0", acquirer.reverseCalls)
	}
	if len(repo.savedTxs) != 0 {
		t.Errorf("saved txs = %d, want 0", len(repo.savedTxs))
	}
}

func TestProcessReversal_ClaimError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	h := command.NewAuthorizationHandler(
		&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, domain.NewTransactionID()))
	if err == nil || !strings.Contains(err.Error(), "claim event") {
		t.Fatalf("error = %v, want it to contain %q", err, "claim event")
	}
}

func TestProcessReversal_InvalidTransactionID(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	h := command.NewAuthorizationHandler(
		&fakeRepo{}, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	cmd := validReversalCmd(t, domain.NewTransactionID())
	cmd.OriginalTransactionID = "not-a-uuid"

	err := h.ProcessReversal(context.Background(), cmd)
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestProcessReversal_FindByIDError(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findErr: errors.New("not found")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, domain.NewTransactionID()))
	if err == nil || !strings.Contains(err.Error(), "find transaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "find transaction")
	}
}

func TestProcessReversal_AcquirerErrorLeavesForReconciliation(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newApprovedTransaction(t)}
	acquirer := &fakeAcquirer{reverseErr: errors.New("acquirer timeout")}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, repo.findResult.ID()))
	if err != nil {
		t.Fatalf("ProcessReversal() error = %v, want nil (acquirer failure is left for reconciliation)", err)
	}
	if len(repo.savedTxs) != 0 {
		t.Errorf("saved txs = %d, want 0", len(repo.savedTxs))
	}
	if publisher.reversalCalls != 0 {
		t.Errorf("PublishReversalCompleted calls = %d, want 0", publisher.reversalCalls)
	}
}

func TestProcessReversal_ReverseAggregateInvalidTransition(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	// tx en RECEIVED: acquirer.Reverse tiene éxito pero tx.Reverse() debe fallar
	// porque RECEIVED no puede transicionar a REVERSED.
	repo := &fakeRepo{findResult: newValidTransaction(t)}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, repo.findResult.ID()))
	if err == nil || !strings.Contains(err.Error(), "reverse aggregate") {
		t.Fatalf("error = %v, want it to contain %q", err, "reverse aggregate")
	}
}

func TestProcessReversal_SaveError(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newApprovedTransaction(t), saveErr: errors.New("db unreachable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, &fakePublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	err := h.ProcessReversal(context.Background(), validReversalCmd(t, repo.findResult.ID()))
	if err == nil || !strings.Contains(err.Error(), "persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "persist")
	}
}

func TestProcessReversal_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newApprovedTransaction(t)}
	acquirer := &fakeAcquirer{}
	publisher := &fakePublisher{}
	h := command.NewAuthorizationHandler(
		repo, acquirer, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ProcessReversal(context.Background(), validReversalCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ProcessReversal() error = %v", err)
	}
	if acquirer.reverseCalls != 1 {
		t.Errorf("Reverse calls = %d, want 1", acquirer.reverseCalls)
	}
	if len(repo.savedTxs) != 1 || repo.savedTxs[0].State() != valueobject.StateReversed {
		t.Fatalf("saved tx = %+v, want a single tx in state REVERSED", repo.savedTxs)
	}
	if publisher.reversalCalls != 1 {
		t.Errorf("PublishReversalCompleted calls = %d, want 1", publisher.reversalCalls)
	}
}

func TestProcessReversal_PublishErrorIsNonFatal(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeRepo{findResult: newApprovedTransaction(t)}
	publisher := &fakePublisher{reversalErr: errors.New("nats unavailable")}
	h := command.NewAuthorizationHandler(
		repo, &fakeAcquirer{}, publisher,
		natsutil.NewIdempotencyStore(nil, idempotencySchema), pool,
	)

	if err := h.ProcessReversal(context.Background(), validReversalCmd(t, repo.findResult.ID())); err != nil {
		t.Fatalf("ProcessReversal() error = %v, want nil (publish failure must not fail the saga)", err)
	}
	if len(repo.savedTxs) != 1 {
		t.Errorf("saved txs = %d, want 1", len(repo.savedTxs))
	}
}
