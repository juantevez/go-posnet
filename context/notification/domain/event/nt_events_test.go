package event_test

import (
	"testing"
	"time"

	"github.com/juantevez/go-posnet/context/notification/domain/event"
	"github.com/juantevez/go-posnet/context/notification/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
)

// withinWindow verifica que ts esté entre before y after, inclusive.
func withinWindow(t *testing.T, ts, before, after time.Time) {
	t.Helper()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp = %v, want between %v and %v", ts, before, after)
	}
}

func TestNewNotificationDispatched(t *testing.T) {
	notificationID := "notif-1"
	txID := domain.NewTransactionID()

	before := time.Now().UTC()
	e := event.NewNotificationDispatched(notificationID, txID, valueobject.ChannelWebhook, 2)
	after := time.Now().UTC()

	if e.NotificationID != notificationID {
		t.Errorf("NotificationID = %q, want %q", e.NotificationID, notificationID)
	}
	if !e.TransactionID.Equals(txID) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if e.Channel != valueobject.ChannelWebhook {
		t.Errorf("Channel = %v, want %v", e.Channel, valueobject.ChannelWebhook)
	}
	if e.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", e.Attempts)
	}
	if e.EventType() != "notification.dispatched" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "notification.dispatched")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

func TestNewNotificationDead(t *testing.T) {
	notificationID := "notif-2"
	txID := domain.NewTransactionID()

	before := time.Now().UTC()
	e := event.NewNotificationDead(notificationID, txID, valueobject.ChannelSMS, 5)
	after := time.Now().UTC()

	if e.NotificationID != notificationID {
		t.Errorf("NotificationID = %q, want %q", e.NotificationID, notificationID)
	}
	if !e.TransactionID.Equals(txID) {
		t.Errorf("TransactionID = %v, want %v", e.TransactionID, txID)
	}
	if e.Channel != valueobject.ChannelSMS {
		t.Errorf("Channel = %v, want %v", e.Channel, valueobject.ChannelSMS)
	}
	if e.TotalAttempts != 5 {
		t.Errorf("TotalAttempts = %d, want 5", e.TotalAttempts)
	}
	if e.EventType() != "notification.dead" {
		t.Errorf("EventType() = %q, want %q", e.EventType(), "notification.dead")
	}
	withinWindow(t, e.OccurredAt(), before, after)
}

// TestEvents_ImplementDomainEventInterface verifica que ambos eventos
// satisfacen la interfaz DomainEvent en tiempo de compilación.
func TestEvents_ImplementDomainEventInterface(t *testing.T) {
	events := []event.DomainEvent{
		event.NewNotificationDispatched("notif-1", domain.NewTransactionID(), valueobject.ChannelEmail, 1),
		event.NewNotificationDead("notif-2", domain.NewTransactionID(), valueobject.ChannelTerminalWebSocket, 5),
	}

	for _, e := range events {
		if e.EventType() == "" {
			t.Errorf("%T.EventType() = \"\", want non-empty", e)
		}
		if e.OccurredAt().IsZero() {
			t.Errorf("%T.OccurredAt() is zero, want set", e)
		}
	}
}
