package pgutil

import (
	"context"
	"strings"
	"testing"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	_, err := NewPool(context.Background(), Config{DSN: "://not-a-valid-dsn"})
	if err == nil || !strings.Contains(err.Error(), "pgutil: parse DSN") {
		t.Fatalf("error = %v, want it to contain %q", err, "pgutil: parse DSN")
	}
}

func TestNewPool_CreatePoolError(t *testing.T) {
	// MaxConns=0 hace que pgxpool.NewWithConfig falle de inmediato
	// ("MaxSize must be >= 1"), sin necesidad de red.
	_, err := NewPool(context.Background(), Config{
		DSN:      "postgres://user:pass@127.0.0.1:5432/db",
		MaxConns: 0,
	})
	if err == nil || !strings.Contains(err.Error(), "pgutil: create pool") {
		t.Fatalf("error = %v, want it to contain %q", err, "pgutil: create pool")
	}
}

func TestNewPool_PingFails(t *testing.T) {
	// Puerto cerrado en loopback — connection refused inmediato, sin
	// depender de una Postgres real disponible.
	_, err := NewPool(context.Background(), Config{
		DSN:      "postgres://user:pass@127.0.0.1:1/db?connect_timeout=1",
		MaxConns: 5,
		MinConns: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "pgutil: ping database") {
		t.Fatalf("error = %v, want it to contain %q", err, "pgutil: ping database")
	}
}

func TestRegisterTypes(t *testing.T) {
	// registerTypes ignora ambos parámetros y siempre devuelve nil — es un
	// punto de extensión para futuros tipos custom de Postgres.
	if err := registerTypes(context.Background(), nil); err != nil {
		t.Fatalf("registerTypes() error = %v, want nil", err)
	}
}
