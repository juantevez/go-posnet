package natsutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/juantevez/go-posnet/pkg/events"
)

type testPayload struct {
	Foo string `json:"foo"`
}

// unmarshalable fuerza un error en json.Marshal — los canales no son serializables.
type unmarshalable struct {
	Ch chan int
}

// ─── Build ────────────────────────────────────────────────────────────────────

func TestBuild_Success(t *testing.T) {
	p := NewPublisher(&fakeJetStream{})

	eventID, data, err := p.Build(context.Background(),
		"posnet.test.event", "posnet.test.event.v1", "agg-1", "Transaction",
		"corr-1", "cause-1", testPayload{Foo: "bar"},
	)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if eventID == "" {
		t.Error("eventID is empty, want a generated UUID")
	}

	envelope, err := events.UnmarshalEnvelope(data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}
	if envelope.EventID != eventID {
		t.Errorf("envelope.EventID = %q, want %q", envelope.EventID, eventID)
	}
	if envelope.EventType != "posnet.test.event.v1" {
		t.Errorf("envelope.EventType = %q, want %q", envelope.EventType, "posnet.test.event.v1")
	}
	if envelope.AggregateID != "agg-1" {
		t.Errorf("envelope.AggregateID = %q, want %q", envelope.AggregateID, "agg-1")
	}
	if envelope.AggregateType != "Transaction" {
		t.Errorf("envelope.AggregateType = %q, want %q", envelope.AggregateType, "Transaction")
	}
	if envelope.CorrelationID != "corr-1" {
		t.Errorf("envelope.CorrelationID = %q, want %q", envelope.CorrelationID, "corr-1")
	}
	if envelope.CausationID != "cause-1" {
		t.Errorf("envelope.CausationID = %q, want %q", envelope.CausationID, "cause-1")
	}

	payload, err := events.Unwrap[testPayload](envelope)
	if err != nil {
		t.Fatalf("Unwrap() error = %v", err)
	}
	if payload.Foo != "bar" {
		t.Errorf("payload.Foo = %q, want %q", payload.Foo, "bar")
	}
}

func TestBuild_WrapError(t *testing.T) {
	p := NewPublisher(&fakeJetStream{})

	_, _, err := p.Build(context.Background(),
		"posnet.test.event", "posnet.test.event.v1", "agg-1", "Transaction",
		"corr-1", "cause-1", unmarshalable{Ch: make(chan int)},
	)
	if err == nil || !strings.Contains(err.Error(), `publisher: wrap event "posnet.test.event.v1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `publisher: wrap event "posnet.test.event.v1"`)
	}
}

// ─── Publish ──────────────────────────────────────────────────────────────────

func TestPublish_Success(t *testing.T) {
	js := &fakeJetStream{ackSeq: 42}
	p := NewPublisher(js)

	seq, err := p.Publish(context.Background(),
		"posnet.test.event", "posnet.test.event.v1", "agg-1", "Transaction",
		"corr-1", "cause-1", testPayload{Foo: "bar"},
	)
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if seq != 42 {
		t.Errorf("seq = %d, want 42", seq)
	}

	if len(js.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(js.published))
	}
	msg := js.published[0]
	if msg.Subject != "posnet.test.event" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "posnet.test.event")
	}

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}
	if got := msg.Header.Get(nats.MsgIdHdr); got != envelope.EventID {
		t.Errorf("Nats-Msg-Id header = %q, want %q (envelope.EventID)", got, envelope.EventID)
	}
}

func TestPublish_WrapError(t *testing.T) {
	js := &fakeJetStream{}
	p := NewPublisher(js)

	_, err := p.Publish(context.Background(),
		"posnet.test.event", "posnet.test.event.v1", "agg-1", "Transaction",
		"corr-1", "cause-1", unmarshalable{Ch: make(chan int)},
	)
	if err == nil || !strings.Contains(err.Error(), `publisher: wrap event "posnet.test.event.v1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `publisher: wrap event "posnet.test.event.v1"`)
	}
	if len(js.published) != 0 {
		t.Errorf("published messages = %d, want 0", len(js.published))
	}
}

func TestPublish_PublishMsgError(t *testing.T) {
	js := &fakeJetStream{publishErr: errors.New("nats unavailable")}
	p := NewPublisher(js)

	_, err := p.Publish(context.Background(),
		"posnet.test.event", "posnet.test.event.v1", "agg-1", "Transaction",
		"corr-1", "cause-1", testPayload{Foo: "bar"},
	)
	if err == nil || !strings.Contains(err.Error(), `publisher: publish to "posnet.test.event"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `publisher: publish to "posnet.test.event"`)
	}
}
