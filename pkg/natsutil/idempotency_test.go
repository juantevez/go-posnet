package natsutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

// ─── IsAlreadyProcessed ────────────────────────────────────────────────────────

func TestIsAlreadyProcessed_True(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT 1 FROM authorization.processed_events").
		WithArgs("evt-1").
		WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))

	s := NewIdempotencyStore(pool, "authorization")
	got, err := s.IsAlreadyProcessed(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("IsAlreadyProcessed() error = %v", err)
	}
	if !got {
		t.Error("IsAlreadyProcessed() = false, want true (row found)")
	}
}

func TestIsAlreadyProcessed_False_NotFound(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT 1 FROM authorization.processed_events").
		WithArgs("evt-1").
		WillReturnError(pgx.ErrNoRows)

	s := NewIdempotencyStore(pool, "authorization")
	got, err := s.IsAlreadyProcessed(context.Background(), "evt-1")
	if err != nil {
		t.Fatalf("IsAlreadyProcessed() error = %v", err)
	}
	if got {
		t.Error("IsAlreadyProcessed() = true, want false (no rows)")
	}
}

func TestIsAlreadyProcessed_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectQuery("SELECT 1 FROM authorization.processed_events").
		WithArgs("evt-1").
		WillReturnError(errors.New("connection reset"))

	s := NewIdempotencyStore(pool, "authorization")
	_, err := s.IsAlreadyProcessed(context.Background(), "evt-1")
	if err == nil || !strings.Contains(err.Error(), `idempotency: check event_id "evt-1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `idempotency: check event_id "evt-1"`)
	}
}

// ─── TryMarkAsProcessed ─────────────────────────────────────────────────────────

func TestTryMarkAsProcessed_NewEvent(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectExec("INSERT INTO authorization.processed_events").
		WithArgs("evt-1").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	pool.ExpectRollback()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pool.Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	s := NewIdempotencyStore(pool, "authorization")
	inserted, err := s.TryMarkAsProcessed(context.Background(), tx, "evt-1")
	if err != nil {
		t.Fatalf("TryMarkAsProcessed() error = %v", err)
	}
	if !inserted {
		t.Error("TryMarkAsProcessed() = false, want true (evento nuevo)")
	}
}

func TestTryMarkAsProcessed_DuplicateEvent(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectExec("INSERT INTO authorization.processed_events").
		WithArgs("evt-1").
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	pool.ExpectRollback()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pool.Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	s := NewIdempotencyStore(pool, "authorization")
	inserted, err := s.TryMarkAsProcessed(context.Background(), tx, "evt-1")
	if err != nil {
		t.Fatalf("TryMarkAsProcessed() error = %v", err)
	}
	if inserted {
		t.Error("TryMarkAsProcessed() = true, want false (evento duplicado — ON CONFLICT DO NOTHING)")
	}
}

func TestTryMarkAsProcessed_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBegin()
	pool.ExpectExec("INSERT INTO authorization.processed_events").
		WithArgs("evt-1").
		WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	tx, err := pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("pool.Begin() error = %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	s := NewIdempotencyStore(pool, "authorization")
	_, err = s.TryMarkAsProcessed(context.Background(), tx, "evt-1")
	if err == nil || !strings.Contains(err.Error(), `idempotency: mark event_id "evt-1"`) {
		t.Fatalf("error = %v, want it to contain %q", err, `idempotency: mark event_id "evt-1"`)
	}
}
