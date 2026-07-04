package nats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func newTestPublisher(js *fakeJetStream) *EventPublisher {
	return NewEventPublisher(natsutil.NewPublisher(js))
}

func TestPublishDispatched_Success(t *testing.T) {
	js := &fakeJetStream{ackSeq: 3}
	p := newTestPublisher(js)
	n := newDeliveredNotification(t)

	before := time.Now().UTC().Add(-time.Second)
	if err := p.PublishDispatched(context.Background(), n); err != nil {
		t.Fatalf("PublishDispatched() error = %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	if len(js.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(js.published))
	}
	msg := js.published[0]
	if msg.Subject != events.SubjectNotificationDispatched {
		t.Errorf("Subject = %q, want %q", msg.Subject, events.SubjectNotificationDispatched)
	}

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}
	if envelope.AggregateType != "Notification" {
		t.Errorf("AggregateType = %q, want %q", envelope.AggregateType, "Notification")
	}
	if envelope.AggregateID != n.ID() {
		t.Errorf("AggregateID = %q, want %q", envelope.AggregateID, n.ID())
	}
	if envelope.CorrelationID != n.TransactionID().String() {
		t.Errorf("CorrelationID = %q, want %q", envelope.CorrelationID, n.TransactionID().String())
	}

	payload, err := events.Unwrap[events.NotificationDispatchedPayload](envelope)
	if err != nil {
		t.Fatalf("Unwrap() error = %v", err)
	}
	if payload.NotificationID != n.ID() {
		t.Errorf("NotificationID = %q, want %q", payload.NotificationID, n.ID())
	}
	if payload.TransactionID != n.TransactionID().String() {
		t.Errorf("TransactionID = %q, want %q", payload.TransactionID, n.TransactionID().String())
	}
	if payload.Channel != n.Channel().String() {
		t.Errorf("Channel = %q, want %q", payload.Channel, n.Channel().String())
	}
	if payload.Attempts != n.AttemptCount() {
		t.Errorf("Attempts = %d, want %d", payload.Attempts, n.AttemptCount())
	}

	dispatchedAt, err := time.Parse(time.RFC3339, payload.DispatchedAt)
	if err != nil {
		t.Fatalf("DispatchedAt %q is not RFC3339: %v", payload.DispatchedAt, err)
	}
	if dispatchedAt.Before(before) || dispatchedAt.After(after) {
		t.Errorf("DispatchedAt = %v, want between %v and %v", dispatchedAt, before, after)
	}
}

func TestPublishDispatched_PublishError(t *testing.T) {
	js := &fakeJetStream{publishErr: errors.New("nats unavailable")}
	p := newTestPublisher(js)
	n := newDeliveredNotification(t)

	err := p.PublishDispatched(context.Background(), n)
	if err == nil || !strings.Contains(err.Error(), "publish NotificationDispatched") {
		t.Fatalf("error = %v, want it to contain %q", err, "publish NotificationDispatched")
	}
}
