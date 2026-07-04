package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"

	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/application/port"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func waitForSaves(t *testing.T, ch chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for save #%d/%d", i+1, n)
		}
	}
}

func validApprovalCmd(t *testing.T) port.NotifyApprovalCommand {
	t.Helper()
	return port.NotifyApprovalCommand{
		EventID:       domain.NewTransactionID().String(),
		TransactionID: domain.NewTransactionID().String(),
		TerminalID:    domain.NewTerminalID().String(),
		MerchantID:    domain.NewMerchantID().String(),
		AuthCode:      "AB1234",
		AmountCents:   5000,
		Currency:      "ARS",
		CardLast4:     "1234",
		CardNetwork:   "VISA",
		EntryMode:     "CHIP",
		AuthorizedAt:  "2026-01-01T10:00:00Z",
	}
}

func validRejectionCmd(t *testing.T) port.NotifyRejectionCommand {
	t.Helper()
	return port.NotifyRejectionCommand{
		EventID:         domain.NewTransactionID().String(),
		TransactionID:   domain.NewTransactionID().String(),
		TerminalID:      domain.NewTerminalID().String(),
		MerchantID:      domain.NewMerchantID().String(),
		RejectionCode:   "05",
		RejectionReason: "Do Not Honor",
		AmountCents:     5000,
		Currency:        "ARS",
		CardLast4:       "1234",
		CardNetwork:     "VISA",
		EntryMode:       "CHIP",
		RejectedAt:      "2026-01-01T10:00:00Z",
	}
}

func validBatchClosedCmd(t *testing.T) port.NotifyBatchClosedCommand {
	t.Helper()
	return port.NotifyBatchClosedCommand{
		EventID:     domain.NewTransactionID().String(),
		BatchID:     domain.NewTransactionID().String(),
		TerminalID:  domain.NewTerminalID().String(),
		MerchantID:  domain.NewMerchantID().String(),
		BatchDate:   "2026-01-01",
		TotalCount:  10,
		TotalAmount: 50000,
		Currency:    "ARS",
		ClosedAt:    "2026-01-01T23:59:59Z",
	}
}

// ─── NotifyApproval ───────────────────────────────────────────────────────────

func TestNotifyApproval_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*port.NotifyApprovalCommand)
		wantErr string
	}{
		{"invalid transaction id", func(c *port.NotifyApprovalCommand) { c.TransactionID = "not-a-uuid" }, "invalid transaction_id"},
		{"invalid merchant id", func(c *port.NotifyApprovalCommand) { c.MerchantID = "not-a-uuid" }, "invalid merchant_id"},
		{"non-positive amount", func(c *port.NotifyApprovalCommand) { c.AmountCents = 0 }, "invalid receipt payload"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validApprovalCmd(t)
			tc.mutate(&cmd)

			h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)
			err := h.NotifyApproval(context.Background(), cmd)
			var ve *pkgerrors.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestNotifyApproval_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeNotificationRepo{saveSignal: make(chan struct{}, 10)}
	terminal := &fakeTerminalNotifier{delivered: true}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	h := command.NewNotifyHandler(repo, terminal, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyApproval(context.Background(), validApprovalCmd(t)); err != nil {
		t.Fatalf("NotifyApproval() error = %v", err)
	}

	// 2 saves síncronos (terminal + webhook) + 2 asíncronos del dispatch.
	waitForSaves(t, repo.saveSignal, 4)
	if repo.savedCount() != 4 {
		t.Errorf("saved count = %d, want 4", repo.savedCount())
	}
}

func TestNotifyApproval_DuplicateEventSkipsPersistAndDispatch(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0)

	repo := &fakeNotificationRepo{}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyApproval(context.Background(), validApprovalCmd(t)); err != nil {
		t.Fatalf("NotifyApproval() error = %v", err)
	}
	if repo.savedCount() != 0 {
		t.Errorf("saved count = %d, want 0 (duplicado no debe persistir ni despachar)", repo.savedCount())
	}
}

func TestNotifyApproval_SaveError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO "+idempotencySchema+".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	repo := &fakeNotificationRepo{saveErr: errors.New("db unreachable")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	err := h.NotifyApproval(context.Background(), validApprovalCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyApproval: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyApproval: persist")
	}
}

func TestNotifyApproval_BeginTxError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)
	err := h.NotifyApproval(context.Background(), validApprovalCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyApproval: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyApproval: persist")
	}
}

func TestNotifyApproval_ClaimExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)
	err := h.NotifyApproval(context.Background(), validApprovalCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyApproval: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyApproval: persist")
	}
}

func TestNotifyApproval_SecondSaveError(t *testing.T) {
	pool := newMockPool(t)
	// El callback falla en el segundo Save, así que la transacción nunca llega
	// a Commit — solo el Rollback diferido (sin Commit, a diferencia de
	// expectClaimed que asume éxito).
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO "+idempotencySchema+".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	// La primera llamada a Save (terminalNotif) tiene éxito; la segunda
	// (webhookNotif) falla — ejercita el segundo `if err != nil` de la
	// transacción, distinto del que ya cubre TestNotifyApproval_SaveError.
	repo := &fakeNotificationRepo{saveErr: errors.New("db unreachable"), saveErrOnCall: 2}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	err := h.NotifyApproval(context.Background(), validApprovalCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyApproval: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyApproval: persist")
	}
	if repo.savedCount() != 2 {
		t.Errorf("saved count = %d, want 2 (ambos Save se intentan, el segundo falla)", repo.savedCount())
	}
}

// ─── NotifyRejection ──────────────────────────────────────────────────────────

func TestNotifyRejection_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*port.NotifyRejectionCommand)
		wantErr string
	}{
		{"invalid transaction id", func(c *port.NotifyRejectionCommand) { c.TransactionID = "not-a-uuid" }, "invalid transaction_id"},
		{"invalid merchant id", func(c *port.NotifyRejectionCommand) { c.MerchantID = "not-a-uuid" }, "invalid merchant_id"},
		{"non-positive amount", func(c *port.NotifyRejectionCommand) { c.AmountCents = 0 }, "invalid receipt payload"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := validRejectionCmd(t)
			tc.mutate(&cmd)

			h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)
			err := h.NotifyRejection(context.Background(), cmd)
			var ve *pkgerrors.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestNotifyRejection_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeNotificationRepo{saveSignal: make(chan struct{}, 10)}
	terminal := &fakeTerminalNotifier{delivered: true}
	h := command.NewNotifyHandler(repo, terminal, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyRejection(context.Background(), validRejectionCmd(t)); err != nil {
		t.Fatalf("NotifyRejection() error = %v", err)
	}

	waitForSaves(t, repo.saveSignal, 2)
	if repo.savedCount() != 2 {
		t.Errorf("saved count = %d, want 2", repo.savedCount())
	}
	if repo.lastSaved().Channel() != valueobject.ChannelTerminalWebSocket {
		t.Errorf("Channel() = %v, want %v (rejections no generan webhook)", repo.lastSaved().Channel(), valueobject.ChannelTerminalWebSocket)
	}
}

func TestNotifyRejection_DuplicateEventSkipsPersistAndDispatch(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0)

	repo := &fakeNotificationRepo{}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyRejection(context.Background(), validRejectionCmd(t)); err != nil {
		t.Fatalf("NotifyRejection() error = %v", err)
	}
	if repo.savedCount() != 0 {
		t.Errorf("saved count = %d, want 0", repo.savedCount())
	}
}

func TestNotifyRejection_SaveError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO "+idempotencySchema+".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	repo := &fakeNotificationRepo{saveErr: errors.New("db unreachable")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	err := h.NotifyRejection(context.Background(), validRejectionCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyRejection: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyRejection: persist")
	}
}

func TestNotifyRejection_ClaimExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)
	err := h.NotifyRejection(context.Background(), validRejectionCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyRejection: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyRejection: persist")
	}
}

// ─── NotifyBatchClosed ────────────────────────────────────────────────────────

func TestNotifyBatchClosed_ValidationError(t *testing.T) {
	cmd := validBatchClosedCmd(t)
	cmd.MerchantID = "not-a-uuid"

	h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)
	err := h.NotifyBatchClosed(context.Background(), cmd)
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestNotifyBatchClosed_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	repo := &fakeNotificationRepo{saveSignal: make(chan struct{}, 10)}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyBatchClosed(context.Background(), validBatchClosedCmd(t)); err != nil {
		t.Fatalf("NotifyBatchClosed() error = %v", err)
	}

	waitForSaves(t, repo.saveSignal, 2)
	if repo.savedCount() != 2 {
		t.Errorf("saved count = %d, want 2", repo.savedCount())
	}
	if repo.lastSaved().Channel() != valueobject.ChannelWebhook {
		t.Errorf("Channel() = %v, want %v", repo.lastSaved().Channel(), valueobject.ChannelWebhook)
	}
}

func TestNotifyBatchClosed_DuplicateEventSkipsPersistAndDispatch(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 0)

	repo := &fakeNotificationRepo{}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	if err := h.NotifyBatchClosed(context.Background(), validBatchClosedCmd(t)); err != nil {
		t.Fatalf("NotifyBatchClosed() error = %v", err)
	}
	if repo.savedCount() != 0 {
		t.Errorf("saved count = %d, want 0", repo.savedCount())
	}
}

func TestNotifyBatchClosed_SaveError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO "+idempotencySchema+".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	repo := &fakeNotificationRepo{saveErr: errors.New("db unreachable")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)

	err := h.NotifyBatchClosed(context.Background(), validBatchClosedCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyBatchClosed: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyBatchClosed: persist")
	}
}

func TestNotifyBatchClosed_ClaimExecError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectExec("INSERT INTO " + idempotencySchema + ".processed_events").
		WithArgs(pgxmock.AnyArg()).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	h := command.NewNotifyHandler(&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), pool)
	err := h.NotifyBatchClosed(context.Background(), validBatchClosedCmd(t))
	if err == nil || !strings.Contains(err.Error(), "NotifyBatchClosed: persist") {
		t.Fatalf("error = %v, want it to contain %q", err, "NotifyBatchClosed: persist")
	}
}

// ─── RetryFailed ──────────────────────────────────────────────────────────────

func TestRetryFailed_FindByIDError(t *testing.T) {
	repo := &fakeNotificationRepo{findErr: errors.New("connection reset")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	err := h.RetryFailed(context.Background(), "notif-1")
	if err == nil || !strings.Contains(err.Error(), "RetryFailed: find notification") {
		t.Fatalf("error = %v, want it to contain %q", err, "RetryFailed: find notification")
	}
}

func TestRetryFailed_NotFound(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: nil}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	err := h.RetryFailed(context.Background(), "notif-999")
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestRetryFailed_Success(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelWebhook)}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	pub := &fakeEventPublisher{}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, pub, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	if repo.savedCount() != 1 {
		t.Fatalf("saved count = %d, want 1", repo.savedCount())
	}
	if repo.lastSaved().State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", repo.lastSaved().State(), valueobject.StateSent)
	}
	if pub.calls() != 1 {
		t.Errorf("PublishDispatched calls = %d, want 1", pub.calls())
	}
}

// ─── dispatch (probado indirectamente vía RetryFailed, que lo llama en forma síncrona) ─

func TestDispatch_TerminalDeliveredMarksSent(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelTerminalWebSocket)}
	terminal := &fakeTerminalNotifier{delivered: true}
	h := command.NewNotifyHandler(repo, terminal, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	if repo.lastSaved().State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", repo.lastSaved().State(), valueobject.StateSent)
	}
}

func TestDispatch_TerminalNotConnectedMarksFailed(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelTerminalWebSocket)}
	terminal := &fakeTerminalNotifier{delivered: false, reason: "no active session"}
	h := command.NewNotifyHandler(repo, terminal, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.StateRetrying {
		t.Fatalf("State() = %v, want %v", saved.State(), valueobject.StateRetrying)
	}
	if !strings.Contains(saved.Attempts()[0].ErrorMessage(), "no active session") {
		t.Errorf("ErrorMessage() = %q, want it to contain %q", saved.Attempts()[0].ErrorMessage(), "no active session")
	}
}

func TestDispatch_TerminalErrorMarksFailed(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelTerminalWebSocket)}
	terminal := &fakeTerminalNotifier{err: errors.New("grpc unavailable")}
	h := command.NewNotifyHandler(repo, terminal, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.StateRetrying {
		t.Fatalf("State() = %v, want %v", saved.State(), valueobject.StateRetrying)
	}
	if !strings.Contains(saved.Attempts()[0].ErrorMessage(), "grpc unavailable") {
		t.Errorf("ErrorMessage() = %q, want it to contain %q", saved.Attempts()[0].ErrorMessage(), "grpc unavailable")
	}
}

func TestDispatch_WebhookNon2xxMarksFailed(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelWebhook)}
	webhook := &fakeWebhookDispatcher{httpStatus: 500}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.StateRetrying {
		t.Fatalf("State() = %v, want %v", saved.State(), valueobject.StateRetrying)
	}
	if !strings.Contains(saved.Attempts()[0].ErrorMessage(), "webhook returned HTTP 500") {
		t.Errorf("ErrorMessage() = %q, want it to contain %q", saved.Attempts()[0].ErrorMessage(), "webhook returned HTTP 500")
	}
}

func TestDispatch_WebhookErrorMarksFailed(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelWebhook)}
	webhook := &fakeWebhookDispatcher{err: errors.New("connection refused")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	saved := repo.lastSaved()
	if saved.State() != valueobject.StateRetrying {
		t.Fatalf("State() = %v, want %v", saved.State(), valueobject.StateRetrying)
	}
}

func TestDispatch_PublishErrorIsNonFatal(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: newPendingNotification(t, valueobject.ChannelWebhook)}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	pub := &fakeEventPublisher{publishErr: errors.New("nats unavailable")}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, pub, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v, want nil", err)
	}
	if repo.lastSaved().State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v (el error de publish no debe impedir el Save)", repo.lastSaved().State(), valueobject.StateSent)
	}
}

func TestDispatch_FinalSaveErrorIsLoggedOnly(t *testing.T) {
	// El Save final de dispatch() solo loguea el error — RetryFailed igual
	// debe devolver nil, ya que dispatch() no retorna nada.
	repo := &fakeNotificationRepo{
		findResult: newPendingNotification(t, valueobject.ChannelWebhook),
		saveErr:    errors.New("db unreachable"),
	}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v, want nil", err)
	}
	if repo.savedCount() != 1 {
		t.Errorf("saved count = %d, want 1 (el Save se intenta aunque falle)", repo.savedCount())
	}
}

func TestDispatch_UnknownChannelSkipsDispatch(t *testing.T) {
	// Reconstitute permite un canal que NotifyHandler nunca crea por su cuenta
	// (solo usa TERMINAL_WEBSOCKET y WEBHOOK) — sirve para ejercitar la rama
	// "default" de dispatch(), que loguea y no llama a Save.
	n := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "notif-1",
		TransactionID: domain.NewTransactionID(),
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelSMS,
		State:         valueobject.StatePending,
		Receipt:       mustReceipt(t),
		MaxAttempts:   5,
		CreatedAt:     time.Now().UTC(),
	})
	repo := &fakeNotificationRepo{findResult: n}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), n.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	if repo.savedCount() != 0 {
		t.Errorf("saved count = %d, want 0 (canal desconocido no debe persistir)", repo.savedCount())
	}
}

func TestDispatch_InvalidStateTransitionSkipsSave(t *testing.T) {
	// Notification ya en estado terminal SENT: dispatch intenta MarkSent de
	// nuevo, que falla (SENT→SENT inválido) — no debe persistir nada.
	n := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "notif-1",
		TransactionID: domain.NewTransactionID(),
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelWebhook,
		State:         valueobject.StateSent,
		Receipt:       mustReceipt(t),
		MaxAttempts:   5,
		CreatedAt:     time.Now().UTC(),
	})
	repo := &fakeNotificationRepo{findResult: n}
	webhook := &fakeWebhookDispatcher{httpStatus: 200}
	h := command.NewNotifyHandler(repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{}, natsutil.NewIdempotencyStore(nil, idempotencySchema), nil)

	if err := h.RetryFailed(context.Background(), n.ID()); err != nil {
		t.Fatalf("RetryFailed() error = %v", err)
	}
	if repo.savedCount() != 0 {
		t.Errorf("saved count = %d, want 0 (transición de estado inválida no debe persistir)", repo.savedCount())
	}
}
