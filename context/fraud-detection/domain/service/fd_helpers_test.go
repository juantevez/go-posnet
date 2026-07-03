package service

import (
	"context"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/aggregate"
	"github.com/juantevez/go-posnet/context/fraud-detection/domain/entity"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── fakeRuleRepo ────────────────────────────────────────────────────────────

type fakeRuleRepo struct {
	rules []*entity.FraudRule
	err   error
}

func (f *fakeRuleRepo) FindAllActive(context.Context) ([]*entity.FraudRule, error) {
	return f.rules, f.err
}

func (f *fakeRuleRepo) Save(context.Context, *entity.FraudRule) error { return nil }

// ─── fakeHistRepo ────────────────────────────────────────────────────────────

type fakeHistRepo struct {
	txPerHour    int
	txPerHourErr error

	avgMerchantAmt    int64
	avgMerchantAmtErr error

	recentRejections    int
	recentRejectionsErr error

	sameAmountCount    int
	sameAmountCountErr error
}

func (f *fakeHistRepo) CountByTerminalLastHour(context.Context, domain.TerminalID) (int, error) {
	return f.txPerHour, f.txPerHourErr
}

func (f *fakeHistRepo) AverageAmountByMerchant(context.Context, domain.MerchantID) (int64, error) {
	return f.avgMerchantAmt, f.avgMerchantAmtErr
}

func (f *fakeHistRepo) CountRecentRejectionsByTerminal(context.Context, domain.TerminalID, int) (int, error) {
	return f.recentRejections, f.recentRejectionsErr
}

func (f *fakeHistRepo) CountSameAmountAttempts(context.Context, domain.TerminalID, int64, int) (int, error) {
	return f.sameAmountCount, f.sameAmountCountErr
}

// ─── builders ────────────────────────────────────────────────────────────────

func mustFraudRule(t *testing.T, id string, scoreWeight int) *entity.FraudRule {
	t.Helper()
	r, err := entity.NewFraudRule(id, "Rule "+id, "description", scoreWeight, 0)
	if err != nil {
		t.Fatalf("NewFraudRule(%q) error = %v", id, err)
	}
	return r
}

func newFraudCase(t *testing.T, amountCents int64, entryMode string) *aggregate.FraudCase {
	t.Helper()
	fc, err := aggregate.NewFraudCase(
		domain.NewTransactionID(),
		domain.NewTerminalID(),
		domain.NewMerchantID(),
		amountCents,
		"ARS",
		"VISA",
		entryMode,
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("NewFraudCase() error = %v", err)
	}
	return fc
}
