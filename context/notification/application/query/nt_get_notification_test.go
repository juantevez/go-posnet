package query_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/application/query"
	"github.com/juantevez/go-posnet/context/notification/domain/aggregate"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
)

// ─── fakeNotificationRepo ────────────────────────────────────────────────────

type fakeNotificationRepo struct {
	findByIDResult *aggregate.Notification
	findByIDErr    error

	findByTxResult []*aggregate.Notification
	findByTxErr    error

	findDeadResult []*aggregate.Notification
	findDeadErr    error
	findDeadLimit  int // último limit recibido
}

func (f *fakeNotificationRepo) Save(context.Context, *aggregate.Notification) error {
	return nil
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

func (f *fakeNotificationRepo) FindDead(_ context.Context, limit int) ([]*aggregate.Notification, error) {
	f.findDeadLimit = limit
	return f.findDeadResult, f.findDeadErr
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

func newNotification(t *testing.T) *aggregate.Notification {
	t.Helper()
	n, err := aggregate.NewNotification(domain.NewTransactionID(), domain.NewMerchantID(), valueobject.ChannelWebhook, mustReceipt(t))
	if err != nil {
		t.Fatalf("NewNotification() error = %v", err)
	}
	return n
}

// ─── GetNotification ─────────────────────────────────────────────────────────

func TestGetNotification_EmptyID(t *testing.T) {
	h := query.NewNotificationQueryHandler(&fakeNotificationRepo{})

	_, err := h.GetNotification(context.Background(), "")
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestGetNotification_RepoError(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDErr: errors.New("connection reset")}
	h := query.NewNotificationQueryHandler(repo)

	_, err := h.GetNotification(context.Background(), "notif-1")
	if err == nil || !containsAll(err.Error(), "GetNotification", "connection reset") {
		t.Fatalf("error = %v, want it to mention GetNotification and the underlying cause", err)
	}
}

func TestGetNotification_NotFound(t *testing.T) {
	repo := &fakeNotificationRepo{findByIDResult: nil}
	h := query.NewNotificationQueryHandler(repo)

	_, err := h.GetNotification(context.Background(), "notif-999")
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestGetNotification_Success(t *testing.T) {
	n := newNotification(t)
	repo := &fakeNotificationRepo{findByIDResult: n}
	h := query.NewNotificationQueryHandler(repo)

	got, err := h.GetNotification(context.Background(), n.ID())
	if err != nil {
		t.Fatalf("GetNotification() error = %v", err)
	}
	if got.ID != n.ID() {
		t.Errorf("ID = %q, want %q", got.ID, n.ID())
	}
	if got.TransactionID != n.TransactionID().String() {
		t.Errorf("TransactionID = %q, want %q", got.TransactionID, n.TransactionID().String())
	}
	if got.MerchantID != n.MerchantID().String() {
		t.Errorf("MerchantID = %q, want %q", got.MerchantID, n.MerchantID().String())
	}
	if got.Channel != n.Channel().String() {
		t.Errorf("Channel = %q, want %q", got.Channel, n.Channel().String())
	}
	if got.State != n.State().String() {
		t.Errorf("State = %q, want %q", got.State, n.State().String())
	}
	if got.AttemptCount != n.AttemptCount() {
		t.Errorf("AttemptCount = %d, want %d", got.AttemptCount, n.AttemptCount())
	}
	if got.MaxAttempts != n.MaxAttempts() {
		t.Errorf("MaxAttempts = %d, want %d", got.MaxAttempts, n.MaxAttempts())
	}
	if got.CreatedAt == "" {
		t.Error("CreatedAt is empty, want an RFC3339 timestamp")
	}
	if got.DispatchedAt != "" {
		t.Errorf("DispatchedAt = %q, want empty (never dispatched)", got.DispatchedAt)
	}
	if got.NextRetryAt != "" {
		t.Errorf("NextRetryAt = %q, want empty (never retried)", got.NextRetryAt)
	}
}

func TestGetNotification_MapsDispatchedAtAndNextRetryAt(t *testing.T) {
	dispatchedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	nextRetryAt := time.Date(2026, 1, 1, 12, 5, 0, 0, time.UTC)
	n := aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:            "notif-1",
		TransactionID: domain.NewTransactionID(),
		MerchantID:    domain.NewMerchantID(),
		Channel:       valueobject.ChannelWebhook,
		State:         valueobject.StateRetrying,
		Receipt:       mustReceipt(t),
		MaxAttempts:   5,
		CreatedAt:     time.Now(),
		DispatchedAt:  &dispatchedAt,
		NextRetryAt:   &nextRetryAt,
	})
	repo := &fakeNotificationRepo{findByIDResult: n}
	h := query.NewNotificationQueryHandler(repo)

	got, err := h.GetNotification(context.Background(), n.ID())
	if err != nil {
		t.Fatalf("GetNotification() error = %v", err)
	}
	if got.DispatchedAt != dispatchedAt.Format(time.RFC3339) {
		t.Errorf("DispatchedAt = %q, want %q", got.DispatchedAt, dispatchedAt.Format(time.RFC3339))
	}
	if got.NextRetryAt != nextRetryAt.Format(time.RFC3339) {
		t.Errorf("NextRetryAt = %q, want %q", got.NextRetryAt, nextRetryAt.Format(time.RFC3339))
	}
}

// ─── GetByTransactionID ──────────────────────────────────────────────────────

func TestGetByTransactionID_InvalidID(t *testing.T) {
	h := query.NewNotificationQueryHandler(&fakeNotificationRepo{})

	_, err := h.GetByTransactionID(context.Background(), "not-a-uuid")
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestGetByTransactionID_RepoError(t *testing.T) {
	repo := &fakeNotificationRepo{findByTxErr: errors.New("connection reset")}
	h := query.NewNotificationQueryHandler(repo)

	txID := domain.NewTransactionID()
	_, err := h.GetByTransactionID(context.Background(), txID.String())
	if err == nil || !containsAll(err.Error(), "GetByTransactionID", "connection reset") {
		t.Fatalf("error = %v, want it to mention GetByTransactionID and the underlying cause", err)
	}
}

func TestGetByTransactionID_Empty(t *testing.T) {
	repo := &fakeNotificationRepo{findByTxResult: nil}
	h := query.NewNotificationQueryHandler(repo)

	txID := domain.NewTransactionID()
	got, err := h.GetByTransactionID(context.Background(), txID.String())
	if err != nil {
		t.Fatalf("GetByTransactionID() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

func TestGetByTransactionID_Success(t *testing.T) {
	n1 := newNotification(t)
	n2 := newNotification(t)
	repo := &fakeNotificationRepo{findByTxResult: []*aggregate.Notification{n1, n2}}
	h := query.NewNotificationQueryHandler(repo)

	got, err := h.GetByTransactionID(context.Background(), n1.TransactionID().String())
	if err != nil {
		t.Fatalf("GetByTransactionID() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != n1.ID() || got[1].ID != n2.ID() {
		t.Errorf("results not mapped in order: got[0].ID=%q got[1].ID=%q", got[0].ID, got[1].ID)
	}
}

// ─── ListDead ─────────────────────────────────────────────────────────────────

func TestListDead_DefaultsLimitWhenNonPositive(t *testing.T) {
	tests := []int{0, -1, -100}
	for _, limit := range tests {
		repo := &fakeNotificationRepo{}
		h := query.NewNotificationQueryHandler(repo)

		if _, err := h.ListDead(context.Background(), limit); err != nil {
			t.Fatalf("ListDead(%d) error = %v", limit, err)
		}
		if repo.findDeadLimit != 50 {
			t.Errorf("ListDead(%d): limit passed to repo = %d, want 50", limit, repo.findDeadLimit)
		}
	}
}

func TestListDead_PassesThroughPositiveLimit(t *testing.T) {
	repo := &fakeNotificationRepo{}
	h := query.NewNotificationQueryHandler(repo)

	if _, err := h.ListDead(context.Background(), 10); err != nil {
		t.Fatalf("ListDead() error = %v", err)
	}
	if repo.findDeadLimit != 10 {
		t.Errorf("limit passed to repo = %d, want 10", repo.findDeadLimit)
	}
}

func TestListDead_RepoError(t *testing.T) {
	repo := &fakeNotificationRepo{findDeadErr: errors.New("connection reset")}
	h := query.NewNotificationQueryHandler(repo)

	_, err := h.ListDead(context.Background(), 10)
	if err == nil || !containsAll(err.Error(), "ListDead", "connection reset") {
		t.Fatalf("error = %v, want it to mention ListDead and the underlying cause", err)
	}
}

func TestListDead_Success(t *testing.T) {
	n1 := newNotification(t)
	n2 := newNotification(t)
	repo := &fakeNotificationRepo{findDeadResult: []*aggregate.Notification{n1, n2}}
	h := query.NewNotificationQueryHandler(repo)

	got, err := h.ListDead(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListDead() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != n1.ID() || got[1].ID != n2.ID() {
		t.Errorf("results not mapped in order: got[0].ID=%q got[1].ID=%q", got[0].ID, got[1].ID)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
