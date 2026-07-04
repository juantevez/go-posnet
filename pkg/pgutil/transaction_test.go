package pgutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestWithTransaction_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectCommit()
	pool.ExpectRollback()

	called := false
	err := WithTransaction(context.Background(), pool, pgx.ReadCommitted, func(tx pgx.Tx) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithTransaction() error = %v", err)
	}
	if !called {
		t.Error("fn was not called")
	}
}

func TestWithTransaction_BeginError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted}).WillReturnError(errors.New("connection refused"))

	err := WithTransaction(context.Background(), pool, pgx.ReadCommitted, func(tx pgx.Tx) error {
		t.Fatal("fn should not be called when BeginTx fails")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "pgutil: begin transaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "pgutil: begin transaction")
	}
}

func TestWithTransaction_FnError_RollsBackUnwrapped(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectRollback()

	fnErr := errors.New("business logic failed")
	err := WithTransaction(context.Background(), pool, pgx.ReadCommitted, func(tx pgx.Tx) error {
		return fnErr
	})
	if !errors.Is(err, fnErr) {
		t.Fatalf("error = %v, want it to be fnErr unwrapped (no pgutil: prefix)", err)
	}
}

func TestWithTransaction_CommitError(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectCommit().WillReturnError(errors.New("connection reset"))
	pool.ExpectRollback()

	err := WithTransaction(context.Background(), pool, pgx.ReadCommitted, func(tx pgx.Tx) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "pgutil: commit transaction") {
		t.Fatalf("error = %v, want it to contain %q", err, "pgutil: commit transaction")
	}
}

func TestWithReadCommitted_UsesReadCommittedIsoLevel(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	pool.ExpectCommit()
	pool.ExpectRollback()

	if err := WithReadCommitted(context.Background(), pool, func(tx pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("WithReadCommitted() error = %v", err)
	}
}

func TestWithRepeatableRead_UsesRepeatableReadIsoLevel(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	pool.ExpectCommit()
	pool.ExpectRollback()

	if err := WithRepeatableRead(context.Background(), pool, func(tx pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("WithRepeatableRead() error = %v", err)
	}
}

func TestWithSerializable_UsesSerializableIsoLevel(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectBeginTx(pgx.TxOptions{IsoLevel: pgx.Serializable})
	pool.ExpectCommit()
	pool.ExpectRollback()

	if err := WithSerializable(context.Background(), pool, func(tx pgx.Tx) error { return nil }); err != nil {
		t.Fatalf("WithSerializable() error = %v", err)
	}
}
