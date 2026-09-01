package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/authorization/domain/valueobject"
	"github.com/juantevez/go-posnet/context/authorization/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

// ─── Save ───────────────────────────────────────────────────────────────────

func TestSave_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectExec("INSERT INTO pn_authorization.transactions").
		WithArgs(anyArgs(21)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := postgres.NewTransactionRepo(pool)
	if err := repo.Save(context.Background(), newValidTransaction(t)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSave_ExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectExec("INSERT INTO pn_authorization.transactions").
		WithArgs(anyArgs(21)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewTransactionRepo(pool)
	err := repo.Save(context.Background(), newValidTransaction(t))
	if err == nil || !strings.Contains(err.Error(), "exec upsert") {
		t.Fatalf("error = %v, want it to contain %q", err, "exec upsert")
	}
}

func TestSave_ApprovedTransactionArgs(t *testing.T) {
	pool := newMockPool(t)
	tx := newApprovedTransaction(t)

	pool.ExpectExec("INSERT INTO pn_authorization.transactions").
		WithArgs(
			tx.ID().String(), tx.TerminalID().String(), tx.MerchantID().String(),
			"APPROVED", tx.Amount().Cents(), tx.Amount().Currency().String(),
			tx.PAN().Last4(), string(tx.PAN().Network()), tx.EntryMode().String(),
			tx.STAN().Value(),
			strPtr(tx.AuthCode().String()), (*string)(nil), (*string)(nil),
			intPtr(tx.FraudDecision().Score), strPtr(tx.FraudDecision().Decision),
			tx.EMVDataBase64(), tx.ISO8583Raw(),
			tx.ReceivedAt(), tx.AuthorizedAt(), (*time.Time)(nil),
			(*string)(nil), // card_token — la transacción de prueba no trae token
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := postgres.NewTransactionRepo(pool)
	if err := repo.Save(context.Background(), tx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSave_RejectedTransactionArgs(t *testing.T) {
	pool := newMockPool(t)
	tx := newRejectedTransaction(t)
	rc := tx.RejectionCode()

	pool.ExpectExec("INSERT INTO pn_authorization.transactions").
		WithArgs(
			tx.ID().String(), tx.TerminalID().String(), tx.MerchantID().String(),
			"REJECTED", tx.Amount().Cents(), tx.Amount().Currency().String(),
			tx.PAN().Last4(), string(tx.PAN().Network()), tx.EntryMode().String(),
			tx.STAN().Value(),
			(*string)(nil), strPtr(rc.Code()), strPtr(string(rc.Source())),
			(*int)(nil), (*string)(nil), // sin fraud decision — Reject() directo desde RECEIVED
			tx.EMVDataBase64(), tx.ISO8583Raw(),
			tx.ReceivedAt(), (*time.Time)(nil), tx.RejectedAt(),
			(*string)(nil), // card_token — la transacción de prueba no trae token
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	repo := postgres.NewTransactionRepo(pool)
	if err := repo.Save(context.Background(), tx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

// ─── FindByID ───────────────────────────────────────────────────────────────

func TestFindByID_Success(t *testing.T) {
	pool := newMockPool(t)
	f := newRowFixture(t)
	f.state = "APPROVED"
	f.authCode = strPtr("AB1234")
	authorizedAt := f.createdAt.Add(time.Minute)
	f.authorizedAt = &authorizedAt
	f.fraudScore = intPtr(10)
	f.fraudDecision = strPtr(valueobject.FraudDecisionApprove)
	rulesJSON, err := json.Marshal([]string{"R1", "R2"})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	f.fraudRulesHit = rulesJSON

	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(f.id).
		WillReturnRows(f.rows())

	repo := postgres.NewTransactionRepo(pool)
	id, err := domain.ParseTransactionID(f.id)
	if err != nil {
		t.Fatalf("ParseTransactionID() error = %v", err)
	}

	tx, err := repo.FindByID(context.Background(), id)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if tx.ID().String() != f.id {
		t.Errorf("ID() = %q, want %q", tx.ID().String(), f.id)
	}
	if tx.State() != valueobject.StateApproved {
		t.Errorf("State() = %v, want %v", tx.State(), valueobject.StateApproved)
	}
	if tx.AuthCode() == nil || tx.AuthCode().String() != "AB1234" {
		t.Errorf("AuthCode() = %v, want AB1234", tx.AuthCode())
	}
	if tx.AuthorizedAt() == nil || !tx.AuthorizedAt().Equal(authorizedAt) {
		t.Errorf("AuthorizedAt() = %v, want %v", tx.AuthorizedAt(), authorizedAt)
	}
	if tx.FraudDecision().Score != 10 || tx.FraudDecision().Decision != valueobject.FraudDecisionApprove {
		t.Errorf("FraudDecision() = %+v, want Score=10 Decision=APPROVE", tx.FraudDecision())
	}
	if len(tx.FraudDecision().RulesHit) != 2 || tx.FraudDecision().RulesHit[0] != "R1" {
		t.Errorf("FraudDecision().RulesHit = %v, want [R1 R2]", tx.FraudDecision().RulesHit)
	}
	if tx.RejectionCode() != nil {
		t.Errorf("RejectionCode() = %v, want nil", tx.RejectionCode())
	}
}

func TestFindByID_NotFound(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(1)...).
		WillReturnError(pgx.ErrNoRows)

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.FindByID(context.Background(), domain.NewTransactionID())
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestFindByID_QueryError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.FindByID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "scanTransaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "scanTransaction")
	}
}

func TestFindByID_InvalidCurrencyInRow(t *testing.T) {
	pool := newMockPool(t)
	f := newRowFixture(t)
	f.currency = "XXX"
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(1)...).
		WillReturnRows(f.rows())

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.FindByID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "parse currency") {
		t.Fatalf("error = %v, want it to contain %q", err, "parse currency")
	}
}

func TestFindByID_InvalidAmountInRow(t *testing.T) {
	pool := newMockPool(t)
	f := newRowFixture(t)
	f.amountCents = 0 // domain.NewMoney rechaza monto cero
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(1)...).
		WillReturnRows(f.rows())

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.FindByID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "parse money") {
		t.Fatalf("error = %v, want it to contain %q", err, "parse money")
	}
}

func TestFindByID_NoFraudDecisionWhenScoreMissing(t *testing.T) {
	pool := newMockPool(t)
	f := newRowFixture(t) // fraudScore/fraudDecision quedan nil
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(1)...).
		WillReturnRows(f.rows())

	repo := postgres.NewTransactionRepo(pool)
	tx, err := repo.FindByID(context.Background(), domain.NewTransactionID())
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if !tx.FraudDecision().IsZero() {
		t.Errorf("FraudDecision() = %+v, want zero value", tx.FraudDecision())
	}
}

// ─── FindBySTAN ─────────────────────────────────────────────────────────────

func TestFindBySTAN_Success(t *testing.T) {
	pool := newMockPool(t)
	f := newRowFixture(t)
	date := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(f.terminalID, f.stan, date).
		WillReturnRows(f.rows())

	repo := postgres.NewTransactionRepo(pool)
	terminalID, err := domain.ParseTerminalID(f.terminalID)
	if err != nil {
		t.Fatalf("ParseTerminalID() error = %v", err)
	}
	tx, err := repo.FindBySTAN(context.Background(), terminalID, mustSTAN(t, f.stan), date)
	if err != nil {
		t.Fatalf("FindBySTAN() error = %v", err)
	}
	if tx.ID().String() != f.id {
		t.Errorf("ID() = %q, want %q", tx.ID().String(), f.id)
	}
}

func TestFindBySTAN_NotFoundReturnsNilNil(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(3)...).
		WillReturnError(pgx.ErrNoRows)

	repo := postgres.NewTransactionRepo(pool)
	tx, err := repo.FindBySTAN(context.Background(), domain.NewTerminalID(), mustSTAN(t, 1), time.Now())
	if err != nil {
		t.Fatalf("FindBySTAN() error = %v, want nil", err)
	}
	if tx != nil {
		t.Errorf("FindBySTAN() tx = %v, want nil", tx)
	}
}

func TestFindBySTAN_OtherErrorPropagates(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM pn_authorization.transactions").
		WithArgs(anyArgs(3)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.FindBySTAN(context.Background(), domain.NewTerminalID(), mustSTAN(t, 1), time.Now())
	if err == nil || !strings.Contains(err.Error(), "scanTransaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "scanTransaction")
	}
}

// ─── UpdateState ────────────────────────────────────────────────────────────

func TestUpdateState_Success(t *testing.T) {
	pool := newMockPool(t)
	id := domain.NewTransactionID()

	pool.ExpectExec("UPDATE pn_authorization.transactions SET state").
		WithArgs("APPROVED", id.String()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	repo := postgres.NewTransactionRepo(pool)
	if err := repo.UpdateState(context.Background(), id, valueobject.StateApproved); err != nil {
		t.Fatalf("UpdateState() error = %v", err)
	}
}

func TestUpdateState_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectExec("UPDATE pn_authorization.transactions SET state").
		WithArgs(anyArgs(2)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewTransactionRepo(pool)
	err := repo.UpdateState(context.Background(), domain.NewTransactionID(), valueobject.StateApproved)
	if err == nil || !strings.Contains(err.Error(), "TransactionRepo.UpdateState") {
		t.Fatalf("error = %v, want it to contain %q", err, "TransactionRepo.UpdateState")
	}
}

// ─── ExistsByID ─────────────────────────────────────────────────────────────

func TestExistsByID_True(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT EXISTS").
		WithArgs(anyArgs(1)...).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))

	repo := postgres.NewTransactionRepo(pool)
	exists, err := repo.ExistsByID(context.Background(), domain.NewTransactionID())
	if err != nil {
		t.Fatalf("ExistsByID() error = %v", err)
	}
	if !exists {
		t.Error("ExistsByID() = false, want true")
	}
}

func TestExistsByID_False(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT EXISTS").
		WithArgs(anyArgs(1)...).
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	repo := postgres.NewTransactionRepo(pool)
	exists, err := repo.ExistsByID(context.Background(), domain.NewTransactionID())
	if err != nil {
		t.Fatalf("ExistsByID() error = %v", err)
	}
	if exists {
		t.Error("ExistsByID() = true, want false")
	}
}

func TestExistsByID_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT EXISTS").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewTransactionRepo(pool)
	_, err := repo.ExistsByID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "TransactionRepo.ExistsByID") {
		t.Fatalf("error = %v, want it to contain %q", err, "TransactionRepo.ExistsByID")
	}
}
