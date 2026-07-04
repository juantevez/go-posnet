package observability

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func restoreMeterProvider(t *testing.T) {
	t.Helper()
	prev := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(prev) })
}

// ─── InitMeter ────────────────────────────────────────────────────────────────

func TestInitMeter_Success(t *testing.T) {
	restoreMeterProvider(t)

	shutdown, err := InitMeter(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("InitMeter() error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown function is nil, want non-nil")
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	if err := shutdown(context.Background()); err != nil {
		t.Errorf("shutdown() error = %v, want nil", err)
	}
}

func TestInitMeter_ChangesGlobalMeterProvider(t *testing.T) {
	restoreMeterProvider(t)
	before := otel.GetMeterProvider()

	shutdown, err := InitMeter(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("InitMeter() error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	after := otel.GetMeterProvider()
	if before == after {
		t.Error("otel.GetMeterProvider() did not change after InitMeter() — expected the global provider to be replaced")
	}
}

// ─── Meter ────────────────────────────────────────────────────────────────────

func TestMeter_ReturnsUsableMeterBeforeInit(t *testing.T) {
	restoreMeterProvider(t)

	m := Meter("posnet.test")
	if m == nil {
		t.Fatal("Meter() returned nil, want a usable (possibly no-op) meter")
	}
	if _, err := m.Int64Counter("posnet_test_counter"); err != nil {
		t.Errorf("Int64Counter() error = %v, want nil even against the default no-op provider", err)
	}
}

func TestMeter_ReturnsUsableMeterAfterInit(t *testing.T) {
	restoreMeterProvider(t)

	shutdown, err := InitMeter(context.Background(), "test-service")
	if err != nil {
		t.Fatalf("InitMeter() error = %v", err)
	}
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	m := Meter("posnet.test")
	counter, err := m.Int64Counter("posnet_test_counter")
	if err != nil {
		t.Fatalf("Int64Counter() error = %v", err)
	}
	counter.Add(context.Background(), 1)
}
