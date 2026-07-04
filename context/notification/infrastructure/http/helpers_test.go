package http

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/repository"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// ─── fakeNotificationRepo ────────────────────────────────────────────────────

type fakeNotificationRepo struct {
	saveErr error

	findByIDResult *aggregate.Notification
	findByIDErr    error

	findByTxResult []*aggregate.Notification
	findByTxErr    error

	findDeadResult    []*aggregate.Notification
	findDeadErr       error
	lastFindDeadLimit int
}

var _ repository.NotificationRepository = (*fakeNotificationRepo)(nil)

func (f *fakeNotificationRepo) Save(context.Context, *aggregate.Notification) error {
	return f.saveErr
}

func (f *fakeNotificationRepo) FindByID(context.Context, string) (*aggregate.Notification, error) {
	return f.findByIDResult, f.findByIDErr
}

func (f *fakeNotificationRepo) FindByTransactionID(context.Context, domain.TransactionID) ([]*aggregate.Notification, error) {
	return f.findByTxResult, f.findByTxErr
}

func (f *fakeNotificationRepo) FindPendingRetries(context.Context, int) ([]*aggregate.Notification, error) {
	return nil, nil
}

// panickyNotificationRepo simula un bug en la capa de aplicación para probar
// que recoverMiddleware efectivamente protege al proceso HTTP.
type panickyNotificationRepo struct{}

var _ repository.NotificationRepository = (*panickyNotificationRepo)(nil)

func (p *panickyNotificationRepo) Save(context.Context, *aggregate.Notification) error { return nil }

func (p *panickyNotificationRepo) FindByID(context.Context, string) (*aggregate.Notification, error) {
	panic("boom")
}

func (p *panickyNotificationRepo) FindByTransactionID(context.Context, domain.TransactionID) ([]*aggregate.Notification, error) {
	return nil, nil
}

func (p *panickyNotificationRepo) FindPendingRetries(context.Context, int) ([]*aggregate.Notification, error) {
	return nil, nil
}

func (p *panickyNotificationRepo) FindDead(context.Context, int) ([]*aggregate.Notification, error) {
	return nil, nil
}

func (f *fakeNotificationRepo) FindDead(_ context.Context, limit int) ([]*aggregate.Notification, error) {
	f.lastFindDeadLimit = limit
	return f.findDeadResult, f.findDeadErr
}

// ─── fakeTerminalNotifier / fakeWebhookDispatcher / fakeEventPublisher ────────

type fakeTerminalNotifier struct {
	delivered bool
	reason    string
	err       error
}

func (f *fakeTerminalNotifier) SendReceipt(context.Context, *aggregate.Notification) (bool, string, error) {
	return f.delivered, f.reason, f.err
}

type fakeWebhookDispatcher struct {
	httpStatus int
	err        error
}

func (f *fakeWebhookDispatcher) Dispatch(context.Context, *aggregate.Notification) (int, error) {
	return f.httpStatus, f.err
}

type fakeEventPublisher struct{}

func (f *fakeEventPublisher) PublishDispatched(context.Context, *aggregate.Notification) error {
	return nil
}

// ─── fakePool ────────────────────────────────────────────────────────────────

// fakePool implementa pgutil.PgxPool. Solo Ping() se ejercita en este
// paquete — ningún handler HTTP del BC Notification abre transacciones.
type fakePool struct {
	pingErr error
}

func (f *fakePool) Ping(context.Context) error { return f.pingErr }

func (f *fakePool) BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error) {
	return nil, errors.New("fakePool: BeginTx not implemented")
}

// ─── builders ────────────────────────────────────────────────────────────────

func mustReceipt(t *testing.T) valueobject.ReceiptPayload {
	t.Helper()
	r, err := valueobject.NewReceiptPayload(
		domain.NewTransactionID().String(), "Merchant Inc", "TERM-001",
		"APPROVED", 5000, "ARS", "1234", "VISA", "CHIP",
		"2026-01-01T10:00:00Z",
	)
	if err != nil {
		t.Fatalf("NewReceiptPayload() error = %v", err)
	}
	return r
}

func newNotification(t *testing.T, txID domain.TransactionID, state valueobject.NotificationState) *aggregate.Notification {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "notif-1",
		TransactionID: txID,
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelWebhook,
		State:         state,
		Receipt:       mustReceipt(t),
		MaxAttempts:   5,
		CreatedAt:     time.Now().UTC(),
	})
}
