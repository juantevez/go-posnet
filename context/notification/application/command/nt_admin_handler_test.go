package command_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func TestForceRetry_EmptyID(t *testing.T) {
	h := command.NewAdminHandler(&fakeNotificationRepo{}, command.NewNotifyHandler(
		&fakeNotificationRepo{}, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	))

	err := h.ForceRetry(context.Background(), "")
	var ve *pkgerrors.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error type = %T, want *pkgerrors.ValidationError", err)
	}
}

func TestForceRetry_FindByIDError(t *testing.T) {
	repo := &fakeNotificationRepo{findErr: errors.New("connection reset")}
	h := command.NewAdminHandler(repo, command.NewNotifyHandler(
		repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	))

	err := h.ForceRetry(context.Background(), "notif-1")
	if err == nil || !strings.Contains(err.Error(), "ForceRetry: find notification") {
		t.Fatalf("error = %v, want it to contain %q", err, "ForceRetry: find notification")
	}
}

func TestForceRetry_NotFound(t *testing.T) {
	repo := &fakeNotificationRepo{findResult: nil}
	h := command.NewAdminHandler(repo, command.NewNotifyHandler(
		repo, &fakeTerminalNotifier{}, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	))

	err := h.ForceRetry(context.Background(), "notif-999")
	var nf *pkgerrors.NotFoundError
	if !errors.As(err, &nf) {
		t.Fatalf("error type = %T, want *pkgerrors.NotFoundError", err)
	}
}

func TestForceRetry_Success(t *testing.T) {
	repo := &fakeNotificationRepo{
		saveSignal: make(chan struct{}, 1),
		findResult: newPendingNotification(t, valueobject.ChannelTerminalWebSocket),
	}
	terminal := &fakeTerminalNotifier{delivered: true}
	notifyHandler := command.NewNotifyHandler(
		repo, terminal, &fakeWebhookDispatcher{}, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	)
	h := command.NewAdminHandler(repo, notifyHandler)

	if err := h.ForceRetry(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("ForceRetry() error = %v", err)
	}

	// dispatch() corre síncronamente dentro de ForceRetry (no en goroutine).
	select {
	case <-repo.saveSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to persist")
	}
	if repo.savedCount() != 1 {
		t.Errorf("saved count = %d, want 1", repo.savedCount())
	}
	if repo.lastSaved().State() != valueobject.StateSent {
		t.Errorf("State() = %v, want %v", repo.lastSaved().State(), valueobject.StateSent)
	}
}

func TestForceRetry_ReturnsNilRegardlessOfDispatchOutcome(t *testing.T) {
	// ForceRetry siempre devuelve nil una vez que encuentra la notificación —
	// el resultado del dispatch interno no afecta el código de retorno.
	repo := &fakeNotificationRepo{
		saveSignal: make(chan struct{}, 1),
		findResult: newPendingNotification(t, valueobject.ChannelWebhook),
	}
	webhook := &fakeWebhookDispatcher{err: errors.New("connection refused")}
	notifyHandler := command.NewNotifyHandler(
		repo, &fakeTerminalNotifier{}, webhook, &fakeEventPublisher{},
		natsutil.NewIdempotencyStore(nil, idempotencySchema), nil,
	)
	h := command.NewAdminHandler(repo, notifyHandler)

	if err := h.ForceRetry(context.Background(), repo.findResult.ID()); err != nil {
		t.Fatalf("ForceRetry() error = %v, want nil", err)
	}

	select {
	case <-repo.saveSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch to persist")
	}
	if repo.lastSaved().State() != valueobject.StateRetrying {
		t.Errorf("State() = %v, want %v (fallo de dispatch → reintento)", repo.lastSaved().State(), valueobject.StateRetrying)
	}
}
