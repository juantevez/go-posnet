package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/settlement/domain/aggregate"
	"github.com/juantevez/go-posnet/context/settlement/domain/repository"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── fakeBatchRepo ───────────────────────────────────────────────────────────

type fakeBatchRepo struct {
	saveErr       error
	saveCallCount int
	savedBatches  []*aggregate.SettlementBatch

	findByIDResult *aggregate.SettlementBatch
	findByIDErr    error

	findOpenResult *aggregate.SettlementBatch
	findOpenErr    error

	findOrCreateResult *aggregate.SettlementBatch
	findOrCreateErr    error

	listResult []*aggregate.SettlementBatch
	listErr    error
}

var _ repository.SettlementBatchRepository = (*fakeBatchRepo)(nil)

func (f *fakeBatchRepo) Save(_ context.Context, b *aggregate.SettlementBatch) error {
	f.saveCallCount++
	f.savedBatches = append(f.savedBatches, b)
	return f.saveErr
}

func (f *fakeBatchRepo) lastSaved() *aggregate.SettlementBatch {
	if len(f.savedBatches) == 0 {
		return nil
	}
	return f.savedBatches[len(f.savedBatches)-1]
}

func (f *fakeBatchRepo) FindByID(context.Context, string) (*aggregate.SettlementBatch, error) {
	return f.findByIDResult, f.findByIDErr
}

func (f *fakeBatchRepo) FindOpenByTerminal(context.Context, domain.TerminalID, time.Time) (*aggregate.SettlementBatch, error) {
	return f.findOpenResult, f.findOpenErr
}

func (f *fakeBatchRepo) FindOrCreateOpen(context.Context, domain.TerminalID, domain.MerchantID, time.Time, string) (*aggregate.SettlementBatch, error) {
	return f.findOrCreateResult, f.findOrCreateErr
}

func (f *fakeBatchRepo) ListByMerchantDate(context.Context, domain.MerchantID, time.Time) ([]*aggregate.SettlementBatch, error) {
	return f.listResult, f.listErr
}

// ─── fakePublisher / fakeProcessor ───────────────────────────────────────────

type fakePublisher struct {
	publishBatchClosedErr   error
	publishBatchClosedCalls int
}

func (f *fakePublisher) PublishBatchClosed(context.Context, *aggregate.SettlementBatch) error {
	f.publishBatchClosedCalls++
	return f.publishBatchClosedErr
}

func (f *fakePublisher) PublishSettlementCompleted(context.Context, string, string, int, int64, int64, string, float64) error {
	return nil
}

type fakeProcessor struct {
	confirmationID string
	err            error
	calls          int
}

func (f *fakeProcessor) Submit(context.Context, *aggregate.SettlementBatch) (string, error) {
	f.calls++
	return f.confirmationID, f.err
}

// ─── builders ────────────────────────────────────────────────────────────────

func newOpenBatch(t *testing.T, terminalID domain.TerminalID, merchantID domain.MerchantID) *aggregate.SettlementBatch {
	t.Helper()
	b, err := aggregate.NewSettlementBatch(terminalID, merchantID, time.Now(), "ARS")
	if err != nil {
		t.Fatalf("NewSettlementBatch() error = %v", err)
	}
	return b
}
