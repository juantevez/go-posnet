package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// ─── StartSpan ────────────────────────────────────────────────────────────────

func TestStartSpan_CreatesNamedSpanWithAttributes(t *testing.T) {
	exporter := setupTestTracing(t)

	_, span := StartSpan(context.Background(), "repository.findByID",
		trace.WithAttributes(attribute.String("db.table", "batches")),
	)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "repository.findByID" {
		t.Errorf("span name = %q, want %q", spans[0].Name, "repository.findByID")
	}
	if v, ok := attrValue(spans[0].Attributes, "db.table"); !ok || v.AsString() != "batches" {
		t.Errorf("db.table attribute = %v, ok=%v, want %q", v, ok, "batches")
	}
	if !spans[0].SpanContext.IsValid() {
		t.Error("span context should be valid with a real tracer provider configured")
	}
}

func TestStartSpan_ChildInheritsParentTrace(t *testing.T) {
	exporter := setupTestTracing(t)

	parentCtx, parentSpan := StartSpan(context.Background(), "parent")
	_, childSpan := StartSpan(parentCtx, "child")
	childSpan.End()
	parentSpan.End()

	spans := exporter.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("exported spans = %d, want 2", len(spans))
	}

	var parentTraceID, parentSpanID string
	var childTraceID, childParentSpanID string
	for _, s := range spans {
		if s.Name == "parent" {
			parentTraceID = s.SpanContext.TraceID().String()
			parentSpanID = s.SpanContext.SpanID().String()
		}
		if s.Name == "child" {
			childTraceID = s.SpanContext.TraceID().String()
			childParentSpanID = s.Parent.SpanID().String()
		}
	}
	if childTraceID != parentTraceID {
		t.Errorf("child trace_id = %q, want it to match parent's %q", childTraceID, parentTraceID)
	}
	if childParentSpanID != parentSpanID {
		t.Errorf("child parent span_id = %q, want it to match parent's span_id %q", childParentSpanID, parentSpanID)
	}
}

// ─── RecordError ──────────────────────────────────────────────────────────────

func TestRecordError_SetsErrorStatusOnSpan(t *testing.T) {
	exporter := setupTestTracing(t)

	ctx, span := StartSpan(context.Background(), "operation")
	wantErr := errors.New("db connection lost")
	RecordError(ctx, wantErr)
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("status code = %v, want %v", spans[0].Status.Code, codes.Error)
	}
	if spans[0].Status.Description != wantErr.Error() {
		t.Errorf("status description = %q, want %q", spans[0].Status.Description, wantErr.Error())
	}
	if len(spans[0].Events) != 1 || spans[0].Events[0].Name != "exception" {
		t.Errorf("events = %+v, want a single %q event (RecordError's default behavior)", spans[0].Events, "exception")
	}
}

func TestRecordError_NoActiveSpan_DoesNotPanic(t *testing.T) {
	setupTestTracing(t)
	// Sin StartSpan previo, trace.SpanFromContext(ctx) devuelve un span
	// no-op — RecordError debe poder llamarse sobre él sin explotar.
	RecordError(context.Background(), errors.New("boom"))
}

// ─── AddEvent ─────────────────────────────────────────────────────────────────

func TestAddEvent_AddsNamedEventWithAttributes(t *testing.T) {
	exporter := setupTestTracing(t)

	ctx, span := StartSpan(context.Background(), "operation")
	AddEvent(ctx, "fraud_check_sent", attribute.String("decision", "approved"))
	span.End()

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if len(spans[0].Events) != 1 {
		t.Fatalf("events = %d, want 1", len(spans[0].Events))
	}
	event := spans[0].Events[0]
	if event.Name != "fraud_check_sent" {
		t.Errorf("event name = %q, want %q", event.Name, "fraud_check_sent")
	}
	if v, ok := attrValue(event.Attributes, "decision"); !ok || v.AsString() != "approved" {
		t.Errorf("decision attribute = %v, ok=%v, want %q", v, ok, "approved")
	}
}

func TestAddEvent_NoActiveSpan_DoesNotPanic(t *testing.T) {
	setupTestTracing(t)
	AddEvent(context.Background(), "some_event")
}

// ─── enrichWithTrace ──────────────────────────────────────────────────────────

func TestEnrichWithTrace_NoActiveSpan_ReturnsSameLogger(t *testing.T) {
	setupTestTracing(t)
	logger := slog.Default()

	got := enrichWithTrace(context.Background(), logger)
	if got != logger {
		t.Error("enrichWithTrace() should return the same logger unchanged when there's no valid span")
	}
}

func TestEnrichWithTrace_ActiveSpan_AddsTraceAndSpanIDs(t *testing.T) {
	setupTestTracing(t)
	ctx, span := StartSpan(context.Background(), "operation")
	defer span.End()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	enriched := enrichWithTrace(ctx, logger)
	enriched.Info("test message")

	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	out := buf.String()
	if !bytes.Contains([]byte(out), []byte("trace_id="+traceID)) {
		t.Errorf("log output = %q, want it to contain trace_id=%s", out, traceID)
	}
	if !bytes.Contains([]byte(out), []byte("span_id="+spanID)) {
		t.Errorf("log output = %q, want it to contain span_id=%s", out, spanID)
	}
}

// ─── InitTracer ───────────────────────────────────────────────────────────────

func TestInitTracer_Success(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	shutdown, err := InitTracer(context.Background(), TracerConfig{
		ServiceName:    "test-service",
		ServiceVersion: "1.0.0",
		OTLPEndpoint:   "127.0.0.1:1", // no hace falta un collector real — el exporter gRPC conecta perezosamente
		Environment:    "test",
	})
	if err != nil {
		t.Fatalf("InitTracer() error = %v", err)
	}
	if shutdown == nil {
		t.Fatal("shutdown function is nil, want non-nil")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		_ = shutdown(ctx)
	})

	// El TracerProvider real debe producir spans con SpanContext válido,
	// a diferencia del no-op por defecto.
	_, span := StartSpan(context.Background(), "post-init-span")
	defer span.End()
	if !span.SpanContext().IsValid() {
		t.Error("span created after InitTracer() should have a valid SpanContext")
	}

	// El propagador compuesto (TraceContext + Baggage) debe haber quedado
	// configurado como global — se verifica inyectando desde un contexto
	// con un span activo (Inject() es un no-op sin uno) y comprobando el
	// header W3C resultante.
	ctxWithSpan, spanForInject := StartSpan(context.Background(), "inject-check")
	defer spanForInject.End()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctxWithSpan, carrier)
	if _, ok := carrier["traceparent"]; !ok {
		t.Error("expected traceparent to be injected by the composite propagator configured by InitTracer")
	}
}
