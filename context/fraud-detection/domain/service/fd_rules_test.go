package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newEngineForRules(t *testing.T) *RuleEngine {
	t.Helper()
	return NewRuleEngine(&fakeRuleRepo{}, &fakeHistRepo{}, time.Second)
}

// ─── RULE-001: velocidad ────────────────────────────────────────────────────

func TestRule001_Velocity(t *testing.T) {
	re := newEngineForRules(t)
	fc := newFraudCase(t, 5000, "CHIP")
	fn := re.evalFns["RULE-001"]

	t.Run("at threshold does not activate", func(t *testing.T) {
		activated, _, err := fn(context.Background(), fc, RuleContext{TxPerHour: 60})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if activated {
			t.Error("activated = true, want false at exactly 60")
		}
	})

	t.Run("above threshold activates", func(t *testing.T) {
		activated, reason, err := fn(context.Background(), fc, RuleContext{TxPerHour: 61})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if !activated {
			t.Fatal("activated = false, want true at 61")
		}
		if reason == "" {
			t.Error("reason is empty, want a description")
		}
	})
}

// ─── RULE-002: monto inusual ────────────────────────────────────────────────

func TestRule002_UnusualAmount(t *testing.T) {
	re := newEngineForRules(t)
	fn := re.evalFns["RULE-002"]

	t.Run("zero average does not activate", func(t *testing.T) {
		fc := newFraudCase(t, 1_000_000, "CHIP")
		activated, _, _ := fn(context.Background(), fc, RuleContext{AvgMerchantAmt: 0})
		if activated {
			t.Error("activated = true, want false when there is no average yet")
		}
	})

	t.Run("exactly 3x average does not activate", func(t *testing.T) {
		fc := newFraudCase(t, 3000, "CHIP")
		activated, _, _ := fn(context.Background(), fc, RuleContext{AvgMerchantAmt: 1000})
		if activated {
			t.Error("activated = true, want false at exactly 3x")
		}
	})

	t.Run("above 3x average activates", func(t *testing.T) {
		fc := newFraudCase(t, 3001, "CHIP")
		activated, _, _ := fn(context.Background(), fc, RuleContext{AvgMerchantAmt: 1000})
		if !activated {
			t.Error("activated = false, want true above 3x")
		}
	})
}

// ─── RULE-003: rechazos recientes ───────────────────────────────────────────

func TestRule003_RecentRejections(t *testing.T) {
	re := newEngineForRules(t)
	fc := newFraudCase(t, 5000, "CHIP")
	fn := re.evalFns["RULE-003"]

	t.Run("at threshold does not activate", func(t *testing.T) {
		activated, _, _ := fn(context.Background(), fc, RuleContext{RecentRejections: 3})
		if activated {
			t.Error("activated = true, want false at exactly 3")
		}
	})

	t.Run("above threshold activates", func(t *testing.T) {
		activated, _, _ := fn(context.Background(), fc, RuleContext{RecentRejections: 4})
		if !activated {
			t.Error("activated = false, want true at 4")
		}
	})
}

// ─── RULE-004: mismo monto repetido ─────────────────────────────────────────

func TestRule004_SameAmountRepeated(t *testing.T) {
	re := newEngineForRules(t)
	fc := newFraudCase(t, 5000, "CHIP")
	fn := re.evalFns["RULE-004"]

	t.Run("at threshold does not activate", func(t *testing.T) {
		activated, _, _ := fn(context.Background(), fc, RuleContext{SameAmountCount: 1})
		if activated {
			t.Error("activated = true, want false at exactly 1")
		}
	})

	t.Run("above threshold activates", func(t *testing.T) {
		activated, _, _ := fn(context.Background(), fc, RuleContext{SameAmountCount: 2})
		if !activated {
			t.Error("activated = false, want true at 2")
		}
	})
}

// ─── RULE-005: magstripe con monto alto ─────────────────────────────────────

func TestRule005_MagstripeHighAmount(t *testing.T) {
	re := newEngineForRules(t)
	fn := re.evalFns["RULE-005"]

	t.Run("non-magstripe never activates", func(t *testing.T) {
		fc := newFraudCase(t, 10_000_000, "CHIP")
		activated, _, _ := fn(context.Background(), fc, RuleContext{})
		if activated {
			t.Error("activated = true, want false for CHIP entry mode")
		}
	})

	t.Run("magstripe at threshold does not activate", func(t *testing.T) {
		fc := newFraudCase(t, 5_000_000, "MAGSTRIPE")
		activated, _, _ := fn(context.Background(), fc, RuleContext{})
		if activated {
			t.Error("activated = true, want false at exactly 5_000_000")
		}
	})

	t.Run("magstripe above threshold activates", func(t *testing.T) {
		fc := newFraudCase(t, 5_000_001, "MAGSTRIPE")
		activated, _, _ := fn(context.Background(), fc, RuleContext{})
		if !activated {
			t.Error("activated = false, want true above 5_000_000")
		}
	})
}

// ─── buildRuleContext ────────────────────────────────────────────────────────

func TestBuildRuleContext_AllQueriesSucceed(t *testing.T) {
	histRepo := &fakeHistRepo{
		txPerHour:        42,
		avgMerchantAmt:   99999,
		recentRejections: 2,
		sameAmountCount:  1,
	}
	re := NewRuleEngine(&fakeRuleRepo{}, histRepo, time.Second)
	fc := newFraudCase(t, 5000, "CHIP")

	ruleCtx, err := re.buildRuleContext(context.Background(), fc)
	if err != nil {
		t.Fatalf("buildRuleContext() error = %v", err)
	}
	if ruleCtx.TxPerHour != 42 {
		t.Errorf("TxPerHour = %d, want 42", ruleCtx.TxPerHour)
	}
	if ruleCtx.AvgMerchantAmt != 99999 {
		t.Errorf("AvgMerchantAmt = %d, want 99999", ruleCtx.AvgMerchantAmt)
	}
	if ruleCtx.RecentRejections != 2 {
		t.Errorf("RecentRejections = %d, want 2", ruleCtx.RecentRejections)
	}
	if ruleCtx.SameAmountCount != 1 {
		t.Errorf("SameAmountCount = %d, want 1", ruleCtx.SameAmountCount)
	}
}

func TestBuildRuleContext_AllQueriesFail(t *testing.T) {
	histRepo := &fakeHistRepo{
		txPerHourErr:        errors.New("db down"),
		avgMerchantAmtErr:   errors.New("db down"),
		recentRejectionsErr: errors.New("db down"),
		sameAmountCountErr:  errors.New("db down"),
	}
	re := NewRuleEngine(&fakeRuleRepo{}, histRepo, time.Second)
	fc := newFraudCase(t, 5000, "CHIP")

	_, err := re.buildRuleContext(context.Background(), fc)
	if err == nil {
		t.Fatal("buildRuleContext() error = nil, want error when all queries fail")
	}
}

func TestBuildRuleContext_PartialFailure(t *testing.T) {
	histRepo := &fakeHistRepo{
		txPerHourErr:     errors.New("db down"), // esta query falla
		avgMerchantAmt:   99999,
		recentRejections: 2,
		sameAmountCount:  1,
	}
	re := NewRuleEngine(&fakeRuleRepo{}, histRepo, time.Second)
	fc := newFraudCase(t, 5000, "CHIP")

	ruleCtx, err := re.buildRuleContext(context.Background(), fc)
	if err != nil {
		t.Fatalf("buildRuleContext() error = %v, want nil (solo una de cuatro queries falló)", err)
	}
	if ruleCtx.TxPerHour != 0 {
		t.Errorf("TxPerHour = %d, want 0 (query falló, queda en su zero value)", ruleCtx.TxPerHour)
	}
	if ruleCtx.AvgMerchantAmt != 99999 {
		t.Errorf("AvgMerchantAmt = %d, want 99999", ruleCtx.AvgMerchantAmt)
	}
}
