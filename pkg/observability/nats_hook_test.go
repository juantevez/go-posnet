package observability

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel/trace"
)

// ─── natsCarrier ──────────────────────────────────────────────────────────────
//
// nats.Header es case-sensitive (a diferencia de http.Header), y el
// propagador W3C de OpenTelemetry usa el literal "traceparent" en
// minúsculas — por eso todos los tests usan esa clave exacta.

func TestNatsCarrier_Get(t *testing.T) {
	header := nats.Header{"traceparent": []string{"value-1"}}
	c := natsCarrier{header: header}

	if got := c.Get("traceparent"); got != "value-1" {
		t.Errorf("Get(traceparent) = %q, want %q", got, "value-1")
	}
	if got := c.Get("missing-key"); got != "" {
		t.Errorf("Get(missing-key) = %q, want empty", got)
	}
}

func TestNatsCarrier_Set(t *testing.T) {
	header := make(nats.Header)
	c := natsCarrier{header: header}
	c.Set("traceparent", "value-1")

	if got := header.Get("traceparent"); got != "value-1" {
		t.Errorf("header.Get(traceparent) = %q, want %q", got, "value-1")
	}
}

func TestNatsCarrier_Keys(t *testing.T) {
	header := nats.Header{"traceparent": []string{"v1"}, "baggage": []string{"v2"}}
	c := natsCarrier{header: header}

	keys := c.Keys()
	if len(keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2", len(keys))
	}
	found := map[string]bool{}
	for _, k := range keys {
		found[k] = true
	}
	if !found["traceparent"] || !found["baggage"] {
		t.Errorf("keys = %v, want to contain traceparent and baggage", keys)
	}
}

// ─── InjectTraceContext ───────────────────────────────────────────────────────

func TestInjectTraceContext_NilHeader_CreatesHeaderAndInjects(t *testing.T) {
	setupTestTracing(t)
	ctx, span := StartSpan(context.Background(), "publish")
	defer span.End()

	msg := &nats.Msg{Subject: "posnet.test.event"}
	InjectTraceContext(ctx, msg)

	if msg.Header == nil {
		t.Fatal("msg.Header is nil, want it created")
	}
	if got := msg.Header.Get("traceparent"); got == "" {
		t.Error("traceparent header was not injected")
	}
}

func TestInjectTraceContext_ExistingHeader_PreservesOtherKeys(t *testing.T) {
	setupTestTracing(t)
	ctx, span := StartSpan(context.Background(), "publish")
	defer span.End()

	msg := &nats.Msg{Subject: "posnet.test.event", Header: nats.Header{"X-Custom": []string{"custom-value"}}}
	InjectTraceContext(ctx, msg)

	if got := msg.Header.Get("X-Custom"); got != "custom-value" {
		t.Errorf("X-Custom header = %q, want %q (no debe perderse al inyectar el trace context)", got, "custom-value")
	}
	if got := msg.Header.Get("traceparent"); got == "" {
		t.Error("traceparent header was not injected")
	}
}

func TestInjectTraceContext_NoActiveSpan_HeaderCreatedButEmpty(t *testing.T) {
	setupTestTracing(t)
	msg := &nats.Msg{Subject: "posnet.test.event"}

	// Sin span activo, el propagador no inyecta nada — pero el header sigue
	// creándose igual (el nil-check corre antes de llamar a Inject()).
	InjectTraceContext(context.Background(), msg)

	if msg.Header == nil {
		t.Fatal("msg.Header is nil, want it created regardless of injection outcome")
	}
	if got := msg.Header.Get("traceparent"); got != "" {
		t.Errorf("traceparent header = %q, want empty (no hay span activo para propagar)", got)
	}
}

// ─── ExtractTraceContext ──────────────────────────────────────────────────────

func TestExtractTraceContext_NilHeader_ReturnsSameContext(t *testing.T) {
	setupTestTracing(t)
	ctx := context.Background()
	msg := &nats.Msg{Subject: "posnet.test.event"}

	got := ExtractTraceContext(ctx, msg)
	if got != ctx {
		t.Error("ExtractTraceContext() should return the same context unchanged when msg.Header is nil")
	}
}

func TestExtractTraceContext_ReconstructsRemoteSpanContext(t *testing.T) {
	setupTestTracing(t)
	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	msg := &nats.Msg{
		Subject: "posnet.test.event",
		Header:  nats.Header{"traceparent": []string{"00-" + incomingTraceID + "-00f067aa0ba902b7-01"}},
	}

	got := ExtractTraceContext(context.Background(), msg)

	sc := trace.SpanContextFromContext(got)
	if !sc.IsValid() {
		t.Fatal("expected a valid span context reconstructed from the traceparent header")
	}
	if sc.TraceID().String() != incomingTraceID {
		t.Errorf("trace_id = %q, want %q", sc.TraceID().String(), incomingTraceID)
	}
	if !sc.IsRemote() {
		t.Error("expected the reconstructed span context to be marked as remote")
	}
}
