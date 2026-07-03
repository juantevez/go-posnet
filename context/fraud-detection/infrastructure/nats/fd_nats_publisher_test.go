package nats

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/juantevez/go-posnet/pkg/domain"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
)

func newTestPublisher(js *fakeJetStream) *EventPublisher {
	return NewEventPublisher(natsutil.NewPublisher(js))
}

func TestPublishFraudScoreCalculated_Success(t *testing.T) {
	js := &fakeJetStream{ackSeq: 7}
	p := newTestPublisher(js)
	txID := domain.NewTransactionID()
	fc := newEvaluatedFraudCase(t, txID)

	// Margen de 1s en cada punta: EvaluatedAt se formatea con time.RFC3339
	// (sin fracción de segundo), así que el valor parseado puede truncar por
	// debajo del "before" capturado con precisión de nanosegundos.
	before := time.Now().UTC().Add(-time.Second)
	if err := p.PublishFraudScoreCalculated(context.Background(), fc); err != nil {
		t.Fatalf("PublishFraudScoreCalculated() error = %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	if len(js.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(js.published))
	}
	msg := js.published[0]
	if msg.Subject != events.SubjectFraudScoreCalculated {
		t.Errorf("Subject = %q, want %q", msg.Subject, events.SubjectFraudScoreCalculated)
	}

	envelope, err := events.UnmarshalEnvelope(msg.Data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}
	if envelope.EventType != events.SubjectFraudScoreCalculated {
		t.Errorf("EventType = %q, want %q", envelope.EventType, events.SubjectFraudScoreCalculated)
	}
	if envelope.AggregateType != "FraudCase" {
		t.Errorf("AggregateType = %q, want %q", envelope.AggregateType, "FraudCase")
	}
	if envelope.AggregateID != txID.String() {
		t.Errorf("AggregateID = %q, want %q", envelope.AggregateID, txID.String())
	}

	payload, err := events.Unwrap[events.FraudScoreCalculatedPayload](envelope)
	if err != nil {
		t.Fatalf("Unwrap() error = %v", err)
	}
	if payload.TransactionID != txID.String() {
		t.Errorf("TransactionID = %q, want %q", payload.TransactionID, txID.String())
	}
	if payload.Score != fc.Score().Score() {
		t.Errorf("Score = %d, want %d", payload.Score, fc.Score().Score())
	}
	if payload.Decision != fc.Score().Decision().String() {
		t.Errorf("Decision = %q, want %q", payload.Decision, fc.Score().Decision().String())
	}
	if len(payload.RulesHit) != 1 || payload.RulesHit[0] != "RULE-001" {
		t.Errorf("RulesHit = %v, want [RULE-001]", payload.RulesHit)
	}

	evaluatedAt, err := time.Parse(time.RFC3339, payload.EvaluatedAt)
	if err != nil {
		t.Fatalf("EvaluatedAt %q is not RFC3339: %v", payload.EvaluatedAt, err)
	}
	if evaluatedAt.Before(before) || evaluatedAt.After(after) {
		t.Errorf("EvaluatedAt = %v, want between %v and %v", evaluatedAt, before, after)
	}
}

func TestPublishFraudScoreCalculated_EmptyRulesHit(t *testing.T) {
	js := &fakeJetStream{}
	p := newTestPublisher(js)
	fc := newApprovedFraudCase(t, domain.NewTransactionID()) // score 0, sin reglas activadas

	if err := p.PublishFraudScoreCalculated(context.Background(), fc); err != nil {
		t.Fatalf("PublishFraudScoreCalculated() error = %v", err)
	}
	if len(js.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(js.published))
	}

	envelope, err := events.UnmarshalEnvelope(js.published[0].Data)
	if err != nil {
		t.Fatalf("UnmarshalEnvelope() error = %v", err)
	}
	payload, err := events.Unwrap[events.FraudScoreCalculatedPayload](envelope)
	if err != nil {
		t.Fatalf("Unwrap() error = %v", err)
	}
	if payload.Score != 0 {
		t.Errorf("Score = %d, want 0", payload.Score)
	}
	if len(payload.RulesHit) != 0 {
		t.Errorf("RulesHit = %v, want empty", payload.RulesHit)
	}
}

func TestPublishFraudScoreCalculated_PublishError(t *testing.T) {
	js := &fakeJetStream{publishErr: errors.New("nats unavailable")}
	p := newTestPublisher(js)
	fc := newEvaluatedFraudCase(t, domain.NewTransactionID())

	err := p.PublishFraudScoreCalculated(context.Background(), fc)
	if err == nil || !strings.Contains(err.Error(), "publish FraudScoreCalculated") {
		t.Fatalf("error = %v, want it to contain %q", err, "publish FraudScoreCalculated")
	}
}
