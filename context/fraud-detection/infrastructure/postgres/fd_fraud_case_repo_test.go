package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/fraud-detection/domain/valueobject"
	"github.com/juantevez/go-posnet/context/fraud-detection/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── Save ───────────────────────────────────────────────────────────────────

func TestFraudCaseRepo_Save_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectExec("INSERT INTO fraud_detection.fraud_cases").
		WithArgs(anyArgs(14)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := postgres.NewFraudCaseRepo(pool)
	if err := repo.Save(context.Background(), newFraudCase(t, domain.NewTransactionID())); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestFraudCaseRepo_Save_ExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectExec("INSERT INTO fraud_detection.fraud_cases").
		WithArgs(anyArgs(14)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewFraudCaseRepo(pool)
	err := repo.Save(context.Background(), newFraudCase(t, domain.NewTransactionID()))
	if err == nil || !strings.Contains(err.Error(), "FraudCaseRepo.Save") {
		t.Fatalf("error = %v, want it to contain %q", err, "FraudCaseRepo.Save")
	}
}

func TestFraudCaseRepo_Save_SerializesRulesHitAndEvaluations(t *testing.T) {
	pool := newMockPool(t)
	txID := domain.NewTransactionID()
	fc := newFraudCase(t, txID)

	wantRulesHit := []string{"RULE-001"}
	wantEvals := []map[string]any{
		{"rule_id": "RULE-001", "activated": true, "score_contribution": 30, "reason": "high velocity"},
	}

	pool.ExpectExec("INSERT INTO fraud_detection.fraud_cases").
		WithArgs(
			fc.ID(), fc.TransactionID().String(),
			fc.TerminalID().String(), fc.MerchantID().String(), fc.AmountCents(), fc.Currency(), fc.CardNetwork(), fc.EntryMode(), fc.OccurredAt(),
			fc.Score().Score(), fc.Score().Decision().String(),
			jsonArg{want: wantRulesHit}, jsonArg{want: wantEvals},
			fc.EvaluatedAt(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := postgres.NewFraudCaseRepo(pool)
	if err := repo.Save(context.Background(), fc); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

// ─── FindByTransactionID ─────────────────────────────────────────────────────

var fraudCaseColumns = []string{
	"id", "transaction_id",
	"terminal_id", "merchant_id", "amount_cents", "currency", "card_network", "entry_mode", "occurred_at",
	"score", "decision", "rules_hit", "evaluations", "evaluated_at",
}

func fraudCaseRow(t *testing.T, txID string) *pgxmock.Rows {
	t.Helper()
	rulesHit := []byte(`["RULE-001"]`)
	evals := []byte(`[{"rule_id":"RULE-001","rule_name":"Velocity","activated":true,"score_contribution":30,"reason":"high velocity"}]`)
	return pgxmock.NewRows(fraudCaseColumns).AddRow(
		"fraud-case-1", txID,
		domain.NewTerminalID().String(), domain.NewMerchantID().String(), int64(5000), "ARS", "VISA", "CHIP",
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		30, "APPROVE", rulesHit, evals,
		time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC),
	)
}

func TestFraudCaseRepo_FindByTransactionID_Success(t *testing.T) {
	pool := newMockPool(t)
	txID := domain.NewTransactionID()

	pool.ExpectQuery("FROM fraud_detection.fraud_cases").
		WithArgs(txID.String()).
		WillReturnRows(fraudCaseRow(t, txID.String()))

	repo := postgres.NewFraudCaseRepo(pool)
	fc, err := repo.FindByTransactionID(context.Background(), txID)
	if err != nil {
		t.Fatalf("FindByTransactionID() error = %v", err)
	}
	if fc == nil {
		t.Fatal("FindByTransactionID() = nil, want a FraudCase")
	}
	if fc.ID() != "fraud-case-1" {
		t.Errorf("ID() = %q, want %q", fc.ID(), "fraud-case-1")
	}
	if !fc.TransactionID().Equals(txID) {
		t.Errorf("TransactionID() = %v, want %v", fc.TransactionID(), txID)
	}
	if fc.Score().Score() != 30 {
		t.Errorf("Score().Score() = %d, want 30", fc.Score().Score())
	}
	if fc.Score().Decision() != valueobject.DecisionApprove {
		t.Errorf("Score().Decision() = %v, want %v", fc.Score().Decision(), valueobject.DecisionApprove)
	}
	if len(fc.Score().RulesHit()) != 1 || fc.Score().RulesHit()[0] != "RULE-001" {
		t.Errorf("Score().RulesHit() = %v, want [RULE-001]", fc.Score().RulesHit())
	}
	if len(fc.Evaluations()) != 1 || fc.Evaluations()[0].RuleID() != "RULE-001" {
		t.Errorf("Evaluations() = %v, want 1 item with RuleID=RULE-001", fc.Evaluations())
	}
	if fc.EvaluatedAt() == nil {
		t.Error("EvaluatedAt() = nil, want non-nil")
	}
}

func TestFraudCaseRepo_FindByTransactionID_NotFound(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_cases").
		WithArgs(anyArgs(1)...).
		WillReturnError(pgx.ErrNoRows)

	repo := postgres.NewFraudCaseRepo(pool)
	fc, err := repo.FindByTransactionID(context.Background(), domain.NewTransactionID())
	if err != nil {
		t.Fatalf("FindByTransactionID() error = %v, want nil", err)
	}
	if fc != nil {
		t.Errorf("FindByTransactionID() = %v, want nil", fc)
	}
}

func TestFraudCaseRepo_FindByTransactionID_QueryError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM fraud_detection.fraud_cases").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewFraudCaseRepo(pool)
	_, err := repo.FindByTransactionID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "FraudCaseRepo.FindByTransactionID") {
		t.Fatalf("error = %v, want it to contain %q", err, "FraudCaseRepo.FindByTransactionID")
	}
}

func TestFraudCaseRepo_FindByTransactionID_InvalidScoreInRow(t *testing.T) {
	pool := newMockPool(t)
	txID := domain.NewTransactionID()
	rows := pgxmock.NewRows(fraudCaseColumns).AddRow(
		"fraud-case-1", txID.String(),
		domain.NewTerminalID().String(), domain.NewMerchantID().String(), int64(5000), "ARS", "VISA", "CHIP",
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		999, "APPROVE", []byte(`[]`), []byte(`[]`), // score fuera de rango [0,100]
		time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC),
	)
	pool.ExpectQuery("FROM fraud_detection.fraud_cases").
		WithArgs(anyArgs(1)...).
		WillReturnRows(rows)

	repo := postgres.NewFraudCaseRepo(pool)
	_, err := repo.FindByTransactionID(context.Background(), txID)
	if err == nil || !strings.Contains(err.Error(), "reconstruct score") {
		t.Fatalf("error = %v, want it to contain %q", err, "reconstruct score")
	}
}

func TestFraudCaseRepo_FindByTransactionID_MalformedJSONColumnsAreIgnored(t *testing.T) {
	pool := newMockPool(t)
	txID := domain.NewTransactionID()
	rows := pgxmock.NewRows(fraudCaseColumns).AddRow(
		"fraud-case-1", txID.String(),
		domain.NewTerminalID().String(), domain.NewMerchantID().String(), int64(5000), "ARS", "VISA", "CHIP",
		time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		30, "APPROVE", []byte("not-json"), []byte("not-json"),
		time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC),
	)
	pool.ExpectQuery("FROM fraud_detection.fraud_cases").
		WithArgs(anyArgs(1)...).
		WillReturnRows(rows)

	repo := postgres.NewFraudCaseRepo(pool)
	fc, err := repo.FindByTransactionID(context.Background(), txID)
	if err != nil {
		t.Fatalf("FindByTransactionID() error = %v, want nil (JSON inválido se ignora silenciosamente)", err)
	}
	if len(fc.Score().RulesHit()) != 0 {
		t.Errorf("Score().RulesHit() = %v, want empty", fc.Score().RulesHit())
	}
	if len(fc.Evaluations()) != 0 {
		t.Errorf("Evaluations() = %v, want empty", fc.Evaluations())
	}
}
