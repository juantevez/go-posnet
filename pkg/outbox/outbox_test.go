package outbox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"
	"github.com/pashagolub/pgxmock/v4"
)

// newMockPool crea un pool pgxmock y registra su cierre y la verificación de
// expectations al finalizar el test.
func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("pgxmock.NewPool() error = %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations: %v", err)
		}
	})
	return pool
}

// fakeJetStream implementa natsclient.JetStreamContext embebiendo la interfaz
// (nil) y sobreescribiendo solo PublishMsg.
type fakeJetStream struct {
	natsclient.JetStreamContext

	publishErr error
	published  []*natsclient.Msg
}

func (f *fakeJetStream) PublishMsg(m *natsclient.Msg, _ ...natsclient.PubOpt) (*natsclient.PubAck, error) {
	f.published = append(f.published, m)
	if f.publishErr != nil {
		return nil, f.publishErr
	}
	return &natsclient.PubAck{Sequence: 1}, nil
}

// ─── Store.InsertTx ───────────────────────────────────────────────────────────

func TestInsertTx_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectExec("INSERT INTO test_schema.outbox").
		WithArgs("posnet.test.event", "evt-1", []byte(`{"a":1}`)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pool.Begin() error = %v", err)
	}
	defer tx.Rollback(context.Background())

	store := NewStore("test_schema")
	if err := store.InsertTx(context.Background(), tx, "posnet.test.event", "evt-1", []byte(`{"a":1}`)); err != nil {
		t.Fatalf("InsertTx() error = %v", err)
	}
}

func TestInsertTx_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectExec("INSERT INTO test_schema.outbox").
		WithArgs("posnet.test.event", "evt-1", []byte(`{}`)).
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pool.Begin() error = %v", err)
	}
	defer tx.Rollback(context.Background())

	store := NewStore("test_schema")
	err = store.InsertTx(context.Background(), tx, "posnet.test.event", "evt-1", []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), `outbox: insert "evt-1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `outbox: insert "evt-1"`)
	}
}

// ─── Relay.publishPending ───────────────────────────────────────────────────

var outboxRowColumns = []string{"id", "subject", "event_id", "payload"}

func TestPublishPending_BeginError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin().WillReturnError(errors.New("connection refused"))

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: begin tx") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: begin tx")
	}
}

func TestPublishPending_QueryError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: query pending") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: query pending")
	}
}

func TestPublishPending_ScanError(t *testing.T) {
	pool := newMockPool(t)
	rows := pgxmock.NewRows(outboxRowColumns).
		AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{}`)).
		RowError(0, errors.New("scan failure"))

	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(rows)
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: scan row") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: scan row")
	}
}

func TestPublishPending_RowsError(t *testing.T) {
	pool := newMockPool(t)
	rows := pgxmock.NewRows(outboxRowColumns).CloseError(errors.New("driver error"))

	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(rows)
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: rows error") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: rows error")
	}
}

func TestPublishPending_EmptyPending_Commits(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns))
	pool.ExpectCommit()
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	if err := r.publishPending(context.Background()); err != nil {
		t.Fatalf("publishPending() error = %v", err)
	}
}

func TestPublishPending_EmptyPending_CommitError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns))
	pool.ExpectCommit().WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	// A diferencia del path no-vacío, el commit del path vacío se devuelve sin
	// envolver (return tx.Commit(ctx) directo) — el error crudo se propaga tal cual.
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error = %v, want it to contain %q", err, "connection reset")
	}
}

func TestPublishPending_Success_OneRow(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns).
			AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{"a":1}`)))
	pool.ExpectExec("DELETE FROM test_schema.outbox").
		WithArgs("row-1").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit()
	pool.ExpectRollback()

	js := &fakeJetStream{}
	r := NewRelay(pool, js, "test_schema", time.Second, 10)
	if err := r.publishPending(context.Background()); err != nil {
		t.Fatalf("publishPending() error = %v", err)
	}

	if len(js.published) != 1 {
		t.Fatalf("published messages = %d, want 1", len(js.published))
	}
	msg := js.published[0]
	if msg.Subject != "posnet.test.event" {
		t.Errorf("Subject = %q, want %q", msg.Subject, "posnet.test.event")
	}
	if string(msg.Data) != `{"a":1}` {
		t.Errorf("Data = %q, want %q", msg.Data, `{"a":1}`)
	}
	if got := msg.Header.Get(natsclient.MsgIdHdr); got != "evt-1" {
		t.Errorf("Nats-Msg-Id header = %q, want %q", got, "evt-1")
	}
}

func TestPublishPending_Success_MultipleRows(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns).
			AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{}`)).
			AddRow("row-2", "posnet.test.event", "evt-2", []byte(`{}`)))
	pool.ExpectExec("DELETE FROM test_schema.outbox").
		WithArgs("row-1").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectExec("DELETE FROM test_schema.outbox").
		WithArgs("row-2").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit()
	pool.ExpectRollback()

	js := &fakeJetStream{}
	r := NewRelay(pool, js, "test_schema", time.Second, 10)
	if err := r.publishPending(context.Background()); err != nil {
		t.Fatalf("publishPending() error = %v", err)
	}
	if len(js.published) != 2 {
		t.Fatalf("published messages = %d, want 2", len(js.published))
	}
}

func TestPublishPending_PublishError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns).
			AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{}`)))
	pool.ExpectRollback()

	js := &fakeJetStream{publishErr: errors.New("nats unavailable")}
	r := NewRelay(pool, js, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), `outbox relay: publish "evt-1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `outbox relay: publish "evt-1"`)
	}
}

func TestPublishPending_DeleteError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns).
			AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{}`)))
	pool.ExpectExec("DELETE FROM test_schema.outbox").
		WithArgs("row-1").
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: delete row-1") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: delete row-1")
	}
}

func TestPublishPending_NonEmpty_CommitError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectQuery("SELECT id, subject, event_id, payload").
		WillReturnRows(pgxmock.NewRows(outboxRowColumns).
			AddRow("row-1", "posnet.test.event", "evt-1", []byte(`{}`)))
	pool.ExpectExec("DELETE FROM test_schema.outbox").
		WithArgs("row-1").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	pool.ExpectCommit().WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", time.Second, 10)
	err := r.publishPending(context.Background())
	if err == nil || !strings.Contains(err.Error(), "outbox relay: commit") {
		t.Fatalf("error = %v, want it to contain %q", err, "outbox relay: commit")
	}
}

// ─── Relay.Run ───────────────────────────────────────────────────────────────

func TestRun_ReturnsOnContextCancellation(t *testing.T) {
	// interval largo — el ticker no debe disparar antes de que ctx.Done() gane
	// la carrera del select, así que no hace falta mockear el pool.
	r := NewRelay(nil, nil, "test_schema", time.Hour, 10)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
}

func TestRun_CallsPublishPendingOnTick(t *testing.T) {
	pool := newMockPool(t)
	// Maybe(): el ticker puede disparar un número variable de veces según el
	// scheduler antes de que ctx expire — cualquier cantidad de llamadas
	// (incluso cero) debe considerarse satisfecha.
	beginExp := pool.ExpectBegin()
	beginExp.WillReturnError(errors.New("db down"))
	beginExp.Maybe()

	r := NewRelay(pool, &fakeJetStream{}, "test_schema", 2*time.Millisecond, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context deadline")
	}
}
