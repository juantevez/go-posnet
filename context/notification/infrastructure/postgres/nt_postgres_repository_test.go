package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/context/notification/infrastructure/postgres"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

// ─── Save ───────────────────────────────────────────────────────────────────

func TestSave_NoAttempts(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO notification.notifications").
		WithArgs(anyArgs(11)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()
	pool.ExpectRollback()

	repo := postgres.NewNotificationRepo(pool)
	if err := repo.Save(context.Background(), newPendingNotification(t)); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSave_WithAttempts(t *testing.T) {
	pool := newMockPool(t)
	n := newSentNotification(t)

	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO notification.notifications").
		WithArgs(anyArgs(11)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectExec("INSERT INTO notification.delivery_attempts").
		WithArgs(anyArgs(7)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()
	pool.ExpectRollback()

	repo := postgres.NewNotificationRepo(pool)
	if err := repo.Save(context.Background(), n); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSave_SerializesReceipt(t *testing.T) {
	pool := newMockPool(t)
	n := newPendingNotification(t)

	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO notification.notifications").
		WithArgs(
			n.ID(), n.TransactionID().String(), n.MerchantID().String(),
			n.Channel().String(), n.State().String(), jsonArg{want: n.Receipt()},
			n.AttemptCount(), n.MaxAttempts(), n.NextRetryAt(),
			n.CreatedAt(), n.DispatchedAt(),
		).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectCommit()
	pool.ExpectRollback()

	repo := postgres.NewNotificationRepo(pool)
	if err := repo.Save(context.Background(), n); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
}

func TestSave_UpsertError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO notification.notifications").
		WithArgs(anyArgs(11)...).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	repo := postgres.NewNotificationRepo(pool)
	err := repo.Save(context.Background(), newPendingNotification(t))
	if err == nil || !strings.Contains(err.Error(), "NotificationRepo.Save: upsert") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotificationRepo.Save: upsert")
	}
}

func TestSave_InsertAttemptError(t *testing.T) {
	pool := newMockPool(t)
	n := newSentNotification(t)

	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO notification.notifications").
		WithArgs(anyArgs(11)...).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectExec("INSERT INTO notification.delivery_attempts").
		WithArgs(anyArgs(7)...).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	repo := postgres.NewNotificationRepo(pool)
	err := repo.Save(context.Background(), n)
	if err == nil || !strings.Contains(err.Error(), "NotificationRepo.Save: insert attempt") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotificationRepo.Save: insert attempt")
	}
}

func TestSave_BeginTxError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	repo := postgres.NewNotificationRepo(pool)
	err := repo.Save(context.Background(), newPendingNotification(t))
	if err == nil || !strings.Contains(err.Error(), "begin transaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "begin transaction")
	}
}

// ─── FindByID ───────────────────────────────────────────────────────────────

func TestFindByID_Success(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.id).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(oneAttemptRow(row.id))

	repo := postgres.NewNotificationRepo(pool)
	n, err := repo.FindByID(context.Background(), row.id)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if n.ID() != row.id {
		t.Errorf("ID() = %q, want %q", n.ID(), row.id)
	}
	if n.TransactionID().String() != row.transactionID {
		t.Errorf("TransactionID() = %q, want %q", n.TransactionID().String(), row.transactionID)
	}
	if n.Channel() != valueobject.ChannelWebhook {
		t.Errorf("Channel() = %v, want %v", n.Channel(), valueobject.ChannelWebhook)
	}
	if n.State() != valueobject.StatePending {
		t.Errorf("State() = %v, want %v", n.State(), valueobject.StatePending)
	}
	if len(n.Attempts()) != 1 {
		t.Fatalf("Attempts() = %v, want 1 item", n.Attempts())
	}
	if n.Attempts()[0].AttemptNumber() != 1 || !n.Attempts()[0].Success() {
		t.Errorf("Attempts()[0] = %+v, want attempt 1 success=true", n.Attempts()[0])
	}
}

func TestFindByID_NotFound(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnError(pgx.ErrNoRows)

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByID(context.Background(), "notif-999")
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestFindByID_ScanError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByID(context.Background(), "notif-1")
	if err == nil || !strings.Contains(err.Error(), "scanNotification") {
		t.Fatalf("error = %v, want it to contain %q", err, "scanNotification")
	}
}

func TestFindByID_LoadAttemptsScanError(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.id).
		WillReturnRows(row.rows())
	// attempt_number con tipo incompatible (string en vez de int) fuerza un
	// error de Scan dentro del loop de loadAttempts.
	badAttempts := pgxmock.NewRows(attemptColumns).AddRow(
		row.id+"-1", row.id, "not-an-int", true, 200, "", time.Now(),
	)
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(badAttempts)

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByID(context.Background(), row.id)
	if err == nil || !strings.Contains(err.Error(), "loadAttempts: scan") {
		t.Fatalf("error = %v, want it to contain %q", err, "loadAttempts: scan")
	}
}

func TestFindByID_LoadAttemptsError(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.id).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByID(context.Background(), row.id)
	if err == nil || !strings.Contains(err.Error(), "loadAttempts") {
		t.Fatalf("error = %v, want it to contain %q", err, "loadAttempts")
	}
}

func TestFindByID_MalformedReceiptIsIgnored(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)
	row.receiptJSON = []byte("not-json")

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.id).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(emptyAttemptRows())

	repo := postgres.NewNotificationRepo(pool)
	n, err := repo.FindByID(context.Background(), row.id)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil (JSON inválido se ignora silenciosamente)", err)
	}
	if n.Receipt() != (valueobject.ReceiptPayload{}) {
		t.Errorf("Receipt() = %+v, want zero value", n.Receipt())
	}
}

func TestFindByID_UnparseableChannelAndState(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)
	row.channel = "BOGUS_CHANNEL"
	row.state = "BOGUS_STATE"

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.id).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(emptyAttemptRows())

	repo := postgres.NewNotificationRepo(pool)
	n, err := repo.FindByID(context.Background(), row.id)
	if err != nil {
		t.Fatalf("FindByID() error = %v, want nil (parse inválido se ignora silenciosamente)", err)
	}
	if n.Channel() != "" {
		t.Errorf("Channel() = %q, want empty (zero value tras parse fallido)", n.Channel())
	}
	if n.State() != "" {
		t.Errorf("State() = %q, want empty (zero value tras parse fallido)", n.State())
	}
}

// ─── FindByTransactionID ─────────────────────────────────────────────────────

func TestFindByTransactionID_Success(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)
	txID, err := domain.ParseTransactionID(row.transactionID)
	if err != nil {
		t.Fatalf("ParseTransactionID() error = %v", err)
	}

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(row.transactionID).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(emptyAttemptRows())

	repo := postgres.NewNotificationRepo(pool)
	results, err := repo.FindByTransactionID(context.Background(), txID)
	if err != nil {
		t.Fatalf("FindByTransactionID() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].ID() != row.id {
		t.Errorf("ID() = %q, want %q", results[0].ID(), row.id)
	}
}

func TestFindByTransactionID_QueryError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByTransactionID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "FindByTransactionID") {
		t.Fatalf("error = %v, want it to contain %q", err, "FindByTransactionID")
	}
}

func TestFindByTransactionID_Empty(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnRows(pgxmock.NewRows(notificationColumns))

	repo := postgres.NewNotificationRepo(pool)
	results, err := repo.FindByTransactionID(context.Background(), domain.NewTransactionID())
	if err != nil {
		t.Fatalf("FindByTransactionID() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("results = %v, want empty", results)
	}
}

func TestFindByTransactionID_LoadAttemptsError(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindByTransactionID(context.Background(), domain.NewTransactionID())
	if err == nil || !strings.Contains(err.Error(), "loadAttempts") {
		t.Fatalf("error = %v, want it to contain %q", err, "loadAttempts")
	}
}

// ─── FindPendingRetries ─────────────────────────────────────────────────────

func TestFindPendingRetries_Success(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)
	row.state = "RETRYING"
	nextRetry := time.Date(2026, 1, 1, 10, 5, 0, 0, time.UTC)
	row.nextRetryAt = &nextRetry

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(10).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(emptyAttemptRows())

	repo := postgres.NewNotificationRepo(pool)
	results, err := repo.FindPendingRetries(context.Background(), 10)
	if err != nil {
		t.Fatalf("FindPendingRetries() error = %v", err)
	}
	if len(results) != 1 || results[0].State() != valueobject.StateRetrying {
		t.Fatalf("results = %+v, want a single RETRYING notification", results)
	}
}

func TestFindPendingRetries_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindPendingRetries(context.Background(), 10)
	if err == nil || !strings.Contains(err.Error(), "FindPendingRetries") {
		t.Fatalf("error = %v, want it to contain %q", err, "FindPendingRetries")
	}
}

// ─── FindDead ────────────────────────────────────────────────────────────────

func TestFindDead_Success(t *testing.T) {
	pool := newMockPool(t)
	row := newNotificationRow(t)
	row.state = "DEAD"

	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(50).
		WillReturnRows(row.rows())
	pool.ExpectQuery("FROM notification.delivery_attempts").
		WithArgs(row.id).
		WillReturnRows(emptyAttemptRows())

	repo := postgres.NewNotificationRepo(pool)
	results, err := repo.FindDead(context.Background(), 50)
	if err != nil {
		t.Fatalf("FindDead() error = %v", err)
	}
	if len(results) != 1 || results[0].State() != valueobject.StateDead {
		t.Fatalf("results = %+v, want a single DEAD notification", results)
	}
}

func TestFindDead_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnError(errors.New("connection reset"))

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindDead(context.Background(), 50)
	if err == nil || !strings.Contains(err.Error(), "FindDead") {
		t.Fatalf("error = %v, want it to contain %q", err, "FindDead")
	}
}

func TestFindDead_ScanErrorMidRow(t *testing.T) {
	pool := newMockPool(t)
	// attempt_count con tipo incompatible (string en vez de int) fuerza un
	// error de Scan dentro del loop de scanAndLoad, distinto del error de
	// Query que ya cubre TestFindDead_Error.
	badRows := pgxmock.NewRows(notificationColumns).AddRow(
		"notif-1", domain.NewTransactionID().String(), domain.NewMerchantID().String(),
		"WEBHOOK", "DEAD", []byte(`{}`), "not-an-int", 5, nil,
		time.Now(), nil,
	)
	pool.ExpectQuery("FROM notification.notifications").
		WithArgs(anyArgs(1)...).
		WillReturnRows(badRows)

	repo := postgres.NewNotificationRepo(pool)
	_, err := repo.FindDead(context.Background(), 50)
	if err == nil || !strings.Contains(err.Error(), "scanNotification") {
		t.Fatalf("error = %v, want it to contain %q", err, "scanNotification")
	}
}
