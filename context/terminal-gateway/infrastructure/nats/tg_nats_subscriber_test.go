package nats

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"

	"github.com/juantevez/go-posnet/context/terminal-gateway/application/command"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/aggregate"
	"github.com/juantevez/go-posnet/context/terminal-gateway/domain/valueobject"
	"github.com/juantevez/go-posnet/pkg/domain"
	pkgerrors "github.com/juantevez/go-posnet/pkg/errors"
	"github.com/juantevez/go-posnet/pkg/events"
	"github.com/juantevez/go-posnet/pkg/natsutil"
	"github.com/juantevez/go-posnet/pkg/outbox"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// ─── logRecorder ─────────────────────────────────────────────────────────────

// logRecorder es un slog.Handler mínimo que guarda los records emitidos, para
// verificar la clasificación permanente/transitoria de nak() sin depender de
// una conexión NATS real.
type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (l *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (l *logRecorder) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, r)
	return nil
}

func (l *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logRecorder) WithGroup(string) slog.Handler      { return l }

func (l *logRecorder) last() slog.Record {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.records[len(l.records)-1]
}

// ─── builders ────────────────────────────────────────────────────────────────

func newTestSessionHandler(pool pgutil.PgxPool, sessionRepo *fakeSessionRepo, notifier *fakeNotifier) *command.SessionHandler {
	return command.NewSessionHandler(
		sessionRepo, nil, notifier, nil,
		natsutil.NewIdempotencyStore(nil, idempotencySchema),
		outbox.NewStore(idempotencySchema),
		pool,
	)
}

func newEnvelopeMsg(t *testing.T, subject, aggregateID string, payload any) *natsclient.Msg {
	t.Helper()
	envelope, err := events.Wrap(subject, aggregateID, "Transaction", aggregateID, "", payload)
	if err != nil {
		t.Fatalf("events.Wrap() error = %v", err)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return &natsclient.Msg{Subject: subject, Data: data}
}

func processingSession(t *testing.T, terminalID domain.TerminalID) *aggregate.PaymentSession {
	t.Helper()
	return aggregate.Reconstitute(aggregate.ReconstituteParams{
		ID:         domain.NewTransactionID(),
		TerminalID: terminalID,
		MerchantID: domain.NewMerchantID(),
		Amount:     mustMoney(t),
		STAN:       mustSTAN(t),
		Channel:    valueobject.ChannelNFC,
		State:      valueobject.StateProcessing,
		ExpiresAt:  time.Now().UTC().Add(5 * time.Minute),
		CreatedAt:  time.Now().UTC(),
	})
}

// ─── Subscribe ──────────────────────────────────────────────────────────────

func TestSubscribe_RegistersConsumer(t *testing.T) {
	js := &fakeJetStream{}
	h := newTestSessionHandler(nil, &fakeSessionRepo{}, &fakeNotifier{})
	s := NewSubscriber(js, h)

	if err := s.Subscribe(); err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if len(js.subscribeCalls) != 1 {
		t.Fatalf("QueueSubscribe calls = %d, want 1", len(js.subscribeCalls))
	}
	if js.subscribeCalls[0].subject != "posnet.auth.>" {
		t.Errorf("subject = %q, want %q", js.subscribeCalls[0].subject, "posnet.auth.>")
	}
	if js.subscribeCalls[0].durable != "gateway-auth-consumer" {
		t.Errorf("durable = %q, want %q", js.subscribeCalls[0].durable, "gateway-auth-consumer")
	}
}

func TestSubscribe_PropagatesRegistrationError(t *testing.T) {
	js := &fakeJetStream{subscribeErr: errors.New("nats unavailable")}
	h := newTestSessionHandler(nil, &fakeSessionRepo{}, &fakeNotifier{})
	s := NewSubscriber(js, h)

	err := s.Subscribe()
	if err == nil || !strings.Contains(err.Error(), "register gateway-auth-consumer") {
		t.Fatalf("error = %v, want it to contain %q", err, "register gateway-auth-consumer")
	}
}

// ─── handleAuthResult ─────────────────────────────────────────────────────────

func TestHandleAuthResult_MalformedEnvelope(t *testing.T) {
	repo := &fakeSessionRepo{}
	h := newTestSessionHandler(nil, repo, &fakeNotifier{})
	s := NewSubscriber(&fakeJetStream{}, h)

	msg := &natsclient.Msg{Subject: "posnet.auth.approved.v1", Data: []byte("not-json")}
	s.handleAuthResult(msg) // no debe hacer panic

	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0", repo.saveCallCount)
	}
}

func TestHandleAuthResult_UnknownEventType(t *testing.T) {
	repo := &fakeSessionRepo{}
	h := newTestSessionHandler(nil, repo, &fakeNotifier{})
	s := NewSubscriber(&fakeJetStream{}, h)

	msg := newEnvelopeMsg(t, "posnet.auth.something-else.v1", "agg-1", map[string]string{"foo": "bar"})
	s.handleAuthResult(msg) // debe ignorarse silenciosamente (Ack), sin panic

	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0", repo.saveCallCount)
	}
}

func TestHandleAuthResult_Approval_UnwrapError(t *testing.T) {
	repo := &fakeSessionRepo{}
	h := newTestSessionHandler(nil, repo, &fakeNotifier{})
	s := NewSubscriber(&fakeJetStream{}, h)

	msg := newEnvelopeMsg(t, events.SubjectAuthApproved, "agg-1", 12345)
	s.handleAuthResult(msg)

	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0 (unwrap fallido debe cortar antes)", repo.saveCallCount)
	}
}

func TestHandleAuthResult_Approval_HandlerError(t *testing.T) {
	repo := &fakeSessionRepo{findErr: errors.New("connection reset")}
	h := newTestSessionHandler(nil, repo, &fakeNotifier{})
	s := NewSubscriber(&fakeJetStream{}, h)

	payload := events.AuthorizationApprovedPayload{
		TransactionID: domain.NewTransactionID().String(),
		TerminalID:    domain.NewTerminalID().String(),
		AuthCode:      "AUTH123",
	}
	msg := newEnvelopeMsg(t, events.SubjectAuthApproved, payload.TransactionID, payload)
	s.handleAuthResult(msg) // no debe hacer panic

	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0", repo.saveCallCount)
	}
}

func TestHandleAuthResult_Approval_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	terminalID := domain.NewTerminalID()
	session := processingSession(t, terminalID)
	repo := &fakeSessionRepo{findResult: session}
	notifier := &fakeNotifier{}
	h := newTestSessionHandler(pool, repo, notifier)
	s := NewSubscriber(&fakeJetStream{}, h)

	payload := events.AuthorizationApprovedPayload{
		TransactionID: session.ID().String(),
		TerminalID:    terminalID.String(),
		AuthCode:      "AUTH123",
	}
	msg := newEnvelopeMsg(t, events.SubjectAuthApproved, payload.TransactionID, payload)
	s.handleAuthResult(msg)

	if repo.saveCallCount != 1 {
		t.Fatalf("Save call count = %d, want 1", repo.saveCallCount)
	}
	if repo.lastSaved().State() != valueobject.StateApproved {
		t.Errorf("State() = %v, want %v", repo.lastSaved().State(), valueobject.StateApproved)
	}
	if notifier.notifyResultCalls != 1 {
		t.Errorf("NotifyResult calls = %d, want 1", notifier.notifyResultCalls)
	}
}

func TestHandleAuthResult_Rejection_UnwrapError(t *testing.T) {
	repo := &fakeSessionRepo{}
	h := newTestSessionHandler(nil, repo, &fakeNotifier{})
	s := NewSubscriber(&fakeJetStream{}, h)

	msg := newEnvelopeMsg(t, events.SubjectAuthRejected, "agg-1", 12345)
	s.handleAuthResult(msg)

	if repo.saveCallCount != 0 {
		t.Errorf("Save call count = %d, want 0", repo.saveCallCount)
	}
}

func TestHandleAuthResult_Rejection_Success(t *testing.T) {
	pool := newMockPool(t)
	expectClaimed(pool, 1)

	terminalID := domain.NewTerminalID()
	session := processingSession(t, terminalID)
	repo := &fakeSessionRepo{findResult: session}
	notifier := &fakeNotifier{}
	h := newTestSessionHandler(pool, repo, notifier)
	s := NewSubscriber(&fakeJetStream{}, h)

	payload := events.AuthorizationRejectedPayload{
		TransactionID:   session.ID().String(),
		TerminalID:      terminalID.String(),
		RejectionCode:   "05",
		RejectionReason: "Do not honor",
		Source:          "ACQUIRER",
	}
	msg := newEnvelopeMsg(t, events.SubjectAuthRejected, payload.TransactionID, payload)
	s.handleAuthResult(msg)

	if repo.saveCallCount != 1 {
		t.Fatalf("Save call count = %d, want 1", repo.saveCallCount)
	}
	if repo.lastSaved().State() != valueobject.StateRejected {
		t.Errorf("State() = %v, want %v", repo.lastSaved().State(), valueobject.StateRejected)
	}
	if notifier.notifyResultCalls != 1 {
		t.Errorf("NotifyResult calls = %d, want 1", notifier.notifyResultCalls)
	}
}

// ─── nak ────────────────────────────────────────────────────────────────────

func TestNak_ValidationErrorTerminatesMessage(t *testing.T) {
	rec := &logRecorder{}
	s := &TG_Subscriber{log: slog.New(rec)}
	msg := &natsclient.Msg{Subject: "test"}

	s.nak(context.Background(), msg, pkgerrors.NewValidationError("bad data"), "evt-1")

	last := rec.last()
	if last.Level != slog.LevelError {
		t.Errorf("level = %v, want Error", last.Level)
	}
	if last.Message != "permanent error — terminating message" {
		t.Errorf("message = %q, want %q", last.Message, "permanent error — terminating message")
	}
}

func TestNak_ConflictErrorTerminatesMessage(t *testing.T) {
	rec := &logRecorder{}
	s := &TG_Subscriber{log: slog.New(rec)}
	msg := &natsclient.Msg{Subject: "test"}

	s.nak(context.Background(), msg, pkgerrors.NewConflictError("evt-1"), "evt-1")

	last := rec.last()
	if last.Level != slog.LevelError {
		t.Errorf("level = %v, want Error", last.Level)
	}
	if last.Message != "permanent error — terminating message" {
		t.Errorf("message = %q, want %q", last.Message, "permanent error — terminating message")
	}
}

func TestNak_TransientErrorRetriesMessage(t *testing.T) {
	rec := &logRecorder{}
	s := &TG_Subscriber{log: slog.New(rec)}
	msg := &natsclient.Msg{Subject: "test"}

	s.nak(context.Background(), msg, errors.New("db timeout"), "evt-2")

	last := rec.last()
	if last.Level != slog.LevelWarn {
		t.Errorf("level = %v, want Warn", last.Level)
	}
	if last.Message != "transient error — nacking for retry" {
		t.Errorf("message = %q, want %q", last.Message, "transient error — nacking for retry")
	}
}
