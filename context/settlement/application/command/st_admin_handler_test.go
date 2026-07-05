package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/application/command"
	"github.com/juantevez/go-posnet/context/settlement/application/port"
	"github.com/juantevez/go-posnet/context/settlement/domain/aggregate"
	"github.com/juantevez/go-posnet/context/settlement/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

func TestForceClose_EmptyBatchID(t *testing.T) {
	h := command.NewAdminHandler(&fakeBatchRepo{}, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "", OperatorID: "op-1"})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestForceClose_EmptyOperatorID(t *testing.T) {
	h := command.NewAdminHandler(&fakeBatchRepo{}, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: ""})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestForceClose_FindByIDError(t *testing.T) {
	repo := &fakeBatchRepo{findByIDErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ForceClose: find batch") {
		t.Fatalf("error = %v, want it to mention ForceClose: find batch", err)
	}
}

func TestForceClose_NotFound(t *testing.T) {
	repo := &fakeBatchRepo{findByIDResult: nil}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-999", OperatorID: "op-1"})
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestForceClose_RequestCloseError(t *testing.T) {
	// Un batch que no está OPEN no puede transicionar a PENDING_CLOSE.
	b := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         "batch-1",
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		BatchDate:  time.Now(),
		State:      valueobject.BatchStateClosed,
		Currency:   "ARS",
		CreatedAt:  time.Now(),
	})
	repo := &fakeBatchRepo{findByIDResult: b}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ForceClose: request close") {
		t.Fatalf("error = %v, want it to mention ForceClose: request close", err)
	}
}

func TestForceClose_SaveError(t *testing.T) {
	b := newOpenBatch(t, domain.NewTerminalID(), domain.NewMerchantID())
	repo := &fakeBatchRepo{findByIDResult: b, saveErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: b.ID(), OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error = %v, want it to propagate the Save error", err)
	}
}

func TestForceClose_Success(t *testing.T) {
	b := newOpenBatch(t, domain.NewTerminalID(), domain.NewMerchantID())
	repo := &fakeBatchRepo{findByIDResult: b}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	if err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: b.ID(), OperatorID: "op-1"}); err != nil {
		t.Fatalf("ForceClose() error = %v", err)
	}

	if repo.saveCallCount != 1 {
		t.Fatalf("Save call count = %d, want 1", repo.saveCallCount)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.BatchStateClosed {
		t.Errorf("State() = %v, want %v", saved.State(), valueobject.BatchStateClosed)
	}
	if saved.Summary() == nil {
		t.Error("Summary() is nil, want it computed by Close()")
	}
}

// ─── ResubmitBatch ────────────────────────────────────────────────────────────

func newClosedBatch(t *testing.T) *aggregate.SettlementBatch {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         "batch-1",
		TerminalID: domain.NewTerminalID(),
		MerchantID: domain.NewMerchantID(),
		BatchDate:  time.Now(),
		State:      valueobject.BatchStateClosed,
		Currency:   "ARS",
		CreatedAt:  time.Now(),
	})
}

func TestResubmitBatch_EmptyBatchID(t *testing.T) {
	h := command.NewAdminHandler(&fakeBatchRepo{}, &fakeProcessor{})

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: "", OperatorID: "op-1"})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestResubmitBatch_EmptyOperatorID(t *testing.T) {
	h := command.NewAdminHandler(&fakeBatchRepo{}, &fakeProcessor{})

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: "batch-1", OperatorID: ""})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestResubmitBatch_FindByIDError(t *testing.T) {
	repo := &fakeBatchRepo{findByIDErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: "batch-1", OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ResubmitBatch: find batch") {
		t.Fatalf("error = %v, want it to mention ResubmitBatch: find batch", err)
	}
}

func TestResubmitBatch_NotFound(t *testing.T) {
	repo := &fakeBatchRepo{findByIDResult: nil}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: "batch-999", OperatorID: "op-1"})
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestResubmitBatch_WrongState(t *testing.T) {
	// Un batch que no está CLOSED no debe poder resubmitirse (ej: OPEN o ya SUBMITTED).
	b := newOpenBatch(t, domain.NewTerminalID(), domain.NewMerchantID())
	repo := &fakeBatchRepo{findByIDResult: b}
	h := command.NewAdminHandler(repo, &fakeProcessor{})

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: b.ID(), OperatorID: "op-1"})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestResubmitBatch_ProcessorError(t *testing.T) {
	b := newClosedBatch(t)
	repo := &fakeBatchRepo{findByIDResult: b}
	processor := &fakeProcessor{err: errors.New("processor unavailable")}
	h := command.NewAdminHandler(repo, processor)

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: b.ID(), OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ResubmitBatch: processor submit") {
		t.Fatalf("error = %v, want it to mention ResubmitBatch: processor submit", err)
	}
	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0 (no debe persistir si el procesador falla)", repo.saveCallCount)
	}
}

func TestResubmitBatch_SaveError(t *testing.T) {
	b := newClosedBatch(t)
	repo := &fakeBatchRepo{findByIDResult: b, saveErr: errors.New("connection reset")}
	processor := &fakeProcessor{confirmationID: "conf-123"}
	h := command.NewAdminHandler(repo, processor)

	err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: b.ID(), OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ResubmitBatch: save") {
		t.Fatalf("error = %v, want it to mention ResubmitBatch: save", err)
	}
}

func TestResubmitBatch_Success(t *testing.T) {
	b := newClosedBatch(t)
	repo := &fakeBatchRepo{findByIDResult: b}
	processor := &fakeProcessor{confirmationID: "conf-123"}
	h := command.NewAdminHandler(repo, processor)

	if err := h.ResubmitBatch(context.Background(), port.ResubmitBatchCommand{BatchID: b.ID(), OperatorID: "op-1"}); err != nil {
		t.Fatalf("ResubmitBatch() error = %v", err)
	}

	if processor.calls != 1 {
		t.Fatalf("processor.Submit calls = %d, want 1", processor.calls)
	}
	if repo.saveCallCount != 1 {
		t.Fatalf("Save call count = %d, want 1", repo.saveCallCount)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.BatchStateSubmitted {
		t.Errorf("State() = %v, want %v", saved.State(), valueobject.BatchStateSubmitted)
	}
}
