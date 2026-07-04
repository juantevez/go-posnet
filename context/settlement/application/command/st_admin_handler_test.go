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
	h := command.NewAdminHandler(&fakeBatchRepo{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "", OperatorID: "op-1"})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestForceClose_EmptyOperatorID(t *testing.T) {
	h := command.NewAdminHandler(&fakeBatchRepo{})

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: ""})
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestForceClose_FindByIDError(t *testing.T) {
	repo := &fakeBatchRepo{findByIDErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo)

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ForceClose: find batch") {
		t.Fatalf("error = %v, want it to mention ForceClose: find batch", err)
	}
}

func TestForceClose_NotFound(t *testing.T) {
	repo := &fakeBatchRepo{findByIDResult: nil}
	h := command.NewAdminHandler(repo)

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
	h := command.NewAdminHandler(repo)

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: "batch-1", OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "ForceClose: request close") {
		t.Fatalf("error = %v, want it to mention ForceClose: request close", err)
	}
}

func TestForceClose_SaveError(t *testing.T) {
	b := newOpenBatch(t, domain.NewTerminalID(), domain.NewMerchantID())
	repo := &fakeBatchRepo{findByIDResult: b, saveErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo)

	err := h.ForceClose(context.Background(), port.ForceCloseCommand{BatchID: b.ID(), OperatorID: "op-1"})
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error = %v, want it to propagate the Save error", err)
	}
}

func TestForceClose_Success(t *testing.T) {
	b := newOpenBatch(t, domain.NewTerminalID(), domain.NewMerchantID())
	repo := &fakeBatchRepo{findByIDResult: b}
	h := command.NewAdminHandler(repo)

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
