package pgutil

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHealthCheck_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectPing()

	if err := HealthCheck(context.Background(), pool); err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
}

func TestHealthCheck_Error(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectPing().WillReturnError(errors.New("connection refused"))

	err := HealthCheck(context.Background(), pool)
	if err == nil || !strings.Contains(err.Error(), "postgres health check failed") {
		t.Fatalf("error = %v, want it to contain %q", err, "postgres health check failed")
	}
}

func TestHealthCheckWithTimeout_Success(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectPing()

	if err := HealthCheckWithTimeout(context.Background(), pool, time.Second); err != nil {
		t.Fatalf("HealthCheckWithTimeout() error = %v", err)
	}
}

func TestHealthCheckWithTimeout_DeadlineExceeded(t *testing.T) {
	pool := newMockPool(t)
	pool.ExpectPing().WillDelayFor(50 * time.Millisecond)

	err := HealthCheckWithTimeout(context.Background(), pool, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "postgres health check failed") {
		t.Fatalf("error = %v, want it to contain %q", err, "postgres health check failed")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want it to wrap context.DeadlineExceeded", err)
	}
}

func TestStats(t *testing.T) {
	// Stats() toma un *pgxpool.Pool concreto — no es mockeable con pgxmock.
	// pgxpool.NewWithConfig es perezoso (no conecta hasta Ping/Acquire), así
	// que se puede construir un pool real sin red para leer sus contadores
	// en cero.
	cfg, err := pgxpool.ParseConfig("postgres://user:pass@127.0.0.1:1/db")
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig() error = %v", err)
	}
	cfg.MaxConns = 9

	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig() error = %v", err)
	}
	defer pool.Close()

	stats := Stats(pool)
	if stats.MaxConns != 9 {
		t.Errorf("MaxConns = %d, want 9", stats.MaxConns)
	}
	if stats.TotalConns != 0 {
		t.Errorf("TotalConns = %d, want 0 (never connected)", stats.TotalConns)
	}
	if stats.AcquiredConns != 0 {
		t.Errorf("AcquiredConns = %d, want 0", stats.AcquiredConns)
	}
	if stats.IdleConns != 0 {
		t.Errorf("IdleConns = %d, want 0", stats.IdleConns)
	}
}
