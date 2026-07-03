package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/fraud-detection/infrastructure/postgres"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

var fraudRuleColumns = []string{"id", "name", "description", "score_weight", "threshold_value", "is_active", "updated_at"}

func fraudRuleRows() *pgxmock.Rows {
	return pgxmock.NewRows(fraudRuleColumns).
		AddRow("RULE-001", "Velocity", "desc", 10, 0.0, true, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)).
		AddRow("RULE-002", "Unusual Amount", "desc", 20, 3.0, true, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}

func TestFraudRuleRepo_FindAllActive_LoadsFromDBOnCacheMiss(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	rules, err := repo.FindAllActive(context.Background())
	if err != nil {
		t.Fatalf("FindAllActive() error = %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("rules = %d, want 2", len(rules))
	}
	if rules[0].ID() != "RULE-001" || rules[0].ScoreWeight() != 10 {
		t.Errorf("rules[0] = %+v, want ID=RULE-001 ScoreWeight=10", rules[0])
	}
	if rules[1].ID() != "RULE-002" || rules[1].ScoreWeight() != 20 {
		t.Errorf("rules[1] = %+v, want ID=RULE-002 ScoreWeight=20", rules[1])
	}
}

func TestFraudRuleRepo_FindAllActive_UsesCacheWithinTTL(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	ctx := context.Background()

	if _, err := repo.FindAllActive(ctx); err != nil {
		t.Fatalf("first FindAllActive() error = %v", err)
	}
	// La segunda llamada, dentro del TTL, no debe volver a golpear la DB —
	// si lo hiciera, pgxmock fallaría por falta de expectation.
	rules, err := repo.FindAllActive(ctx)
	if err != nil {
		t.Fatalf("second FindAllActive() error = %v", err)
	}
	if len(rules) != 2 {
		t.Errorf("rules = %d, want 2 (desde cache)", len(rules))
	}
}

func TestFraudRuleRepo_FindAllActive_ReloadsAfterTTLExpires(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())

	repo := postgres.NewFraudRuleRepo(pool, time.Nanosecond) // TTL casi nulo
	ctx := context.Background()

	if _, err := repo.FindAllActive(ctx); err != nil {
		t.Fatalf("first FindAllActive() error = %v", err)
	}
	time.Sleep(time.Millisecond) // asegurar que el TTL expiró
	if _, err := repo.FindAllActive(ctx); err != nil {
		t.Fatalf("second FindAllActive() error = %v", err)
	}
}

func TestFraudRuleRepo_FindAllActive_QueryError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnError(errors.New("connection reset"))

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	_, err := repo.FindAllActive(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FraudRuleRepo.loadFromDB") {
		t.Fatalf("error = %v, want it to contain %q", err, "FraudRuleRepo.loadFromDB")
	}
}

func TestFraudRuleRepo_FindAllActive_ScanError(t *testing.T) {
	pool := newMockPool(t)
	// score_weight con tipo incompatible (string en vez de int) para forzar un
	// error de Scan.
	rows := pgxmock.NewRows(fraudRuleColumns).
		AddRow("RULE-001", "Velocity", "desc", "not-an-int", 0.0, true, time.Now())
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(rows)

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	_, err := repo.FindAllActive(context.Background())
	if err == nil || !strings.Contains(err.Error(), "scan row") {
		t.Fatalf("error = %v, want it to contain %q", err, "scan row")
	}
}

func TestFraudRuleRepo_Save_Success(t *testing.T) {
	pool := newMockPool(t)
	rule := mustFraudRule(t, "RULE-001", 50)

	pool.ExpectExec("UPDATE fraud_detection.fraud_rules").
		WithArgs(rule.ScoreWeight(), rule.ThresholdValue(), rule.IsActive(), rule.ID()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	if err := repo.Save(context.Background(), rule); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestFraudRuleRepo_Save_NotFound(t *testing.T) {
	pool := newMockPool(t)
	rule := mustFraudRule(t, "RULE-999", 50)

	pool.ExpectExec("UPDATE fraud_detection.fraud_rules").
		WithArgs(anyArgs(4)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	err := repo.Save(context.Background(), rule)
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestFraudRuleRepo_Save_ExecError(t *testing.T) {
	pool := newMockPool(t)
	rule := mustFraudRule(t, "RULE-001", 50)

	pool.ExpectExec("UPDATE fraud_detection.fraud_rules").
		WithArgs(anyArgs(4)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	err := repo.Save(context.Background(), rule)
	if err == nil || !strings.Contains(err.Error(), "FraudRuleRepo.Save") {
		t.Fatalf("error = %v, want it to contain %q", err, "FraudRuleRepo.Save")
	}
}

func TestFraudRuleRepo_Save_InvalidatesCache(t *testing.T) {
	pool := newMockPool(t)
	rule := mustFraudRule(t, "RULE-001", 50)

	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())
	pool.ExpectExec("UPDATE fraud_detection.fraud_rules").
		WithArgs(anyArgs(4)...).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	pool.ExpectQuery("FROM fraud_detection.fraud_rules").WillReturnRows(fraudRuleRows())

	repo := postgres.NewFraudRuleRepo(pool, time.Minute)
	ctx := context.Background()

	if _, err := repo.FindAllActive(ctx); err != nil {
		t.Fatalf("FindAllActive() error = %v", err)
	}
	if err := repo.Save(ctx, rule); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	// Tras Save, el cache fue invalidado — esta llamada debe volver a golpear
	// la DB (si no lo hiciera, pgxmock fallaría por expectation sin cumplir).
	if _, err := repo.FindAllActive(ctx); err != nil {
		t.Fatalf("FindAllActive() after Save() error = %v", err)
	}
}
