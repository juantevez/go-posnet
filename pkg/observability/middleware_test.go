package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcstatus "google.golang.org/grpc/status"
)

// setupTestTracing instala un TracerProvider real (con exportador en memoria)
// y el propagador W3C TraceContext como globales de OpenTelemetry, y restaura
// los anteriores al finalizar el test. Necesario porque el tracer no-op por
// defecto produce SpanContexts inválidos, con los que el propagador no
// inyecta/extrae nada — hace falta un SDK real para probar la propagación.
func setupTestTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))

	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})
	return exporter
}

func attrValue(attrs []attribute.KeyValue, key string) (attribute.Value, bool) {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value, true
		}
	}
	return attribute.Value{}, false
}

// ─── HTTPMiddleware ───────────────────────────────────────────────────────────

func TestHTTPMiddleware_CapturesStatusCode(t *testing.T) {
	setupTestTracing(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	handler := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodPost, "/batches", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestHTTPMiddleware_DefaultsToStatusOK(t *testing.T) {
	setupTestTracing(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok")) // nunca llama WriteHeader explícitamente
	})
	handler := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHTTPMiddleware_EnrichesContextWithLogger(t *testing.T) {
	setupTestTracing(t)
	var loggerSeen bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, loggerSeen = r.Context().Value(loggerKey).(*slog.Logger)
	})
	handler := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/batches/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !loggerSeen {
		t.Error("loggerKey not present in request context, or next handler was not invoked")
	}
}

func TestHTTPMiddleware_RecordsSpanAttributes(t *testing.T) {
	exporter := setupTestTracing(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	handler := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/batches/999", nil)
	req.Host = "settlement.internal"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	span := spans[0]

	if span.Name != "GET /batches/999" {
		t.Errorf("span name = %q, want %q", span.Name, "GET /batches/999")
	}
	if v, ok := attrValue(span.Attributes, "http.method"); !ok || v.AsString() != "GET" {
		t.Errorf("http.method attribute = %v, ok=%v, want GET", v, ok)
	}
	if v, ok := attrValue(span.Attributes, "http.host"); !ok || v.AsString() != "settlement.internal" {
		t.Errorf("http.host attribute = %v, ok=%v, want settlement.internal", v, ok)
	}
	if v, ok := attrValue(span.Attributes, "http.status_code"); !ok || v.AsInt64() != http.StatusNotFound {
		t.Errorf("http.status_code attribute = %v, ok=%v, want %d", v, ok, http.StatusNotFound)
	}
	if _, ok := attrValue(span.Attributes, "http.duration_ms"); !ok {
		t.Error("http.duration_ms attribute is missing")
	}
}

func TestHTTPMiddleware_ExtractsIncomingTraceParent(t *testing.T) {
	exporter := setupTestTracing(t)

	// traceparent con un trace_id conocido, generado siguiendo el formato W3C:
	// version-traceid-spanid-flags.
	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	handler := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/batches/1", nil)
	req.Header.Set("traceparent", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != incomingTraceID {
		t.Errorf("propagated trace_id = %q, want %q (debe heredar el trace entrante)", got, incomingTraceID)
	}
}

// ─── GRPCUnaryServerInterceptor ───────────────────────────────────────────────

func TestGRPCUnaryServerInterceptor_Success(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/posnet.Settlement/GetBatch"}

	wantResp := "response"
	handler := func(ctx context.Context, req any) (any, error) {
		return wantResp, nil
	}

	resp, err := interceptor(context.Background(), "request", info, handler)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if resp != wantResp {
		t.Errorf("resp = %v, want %v", resp, wantResp)
	}
}

func TestGRPCUnaryServerInterceptor_EnrichesContextWithLogger(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/posnet.Settlement/GetBatch"}

	var sawLogger bool
	handler := func(ctx context.Context, req any) (any, error) {
		_, sawLogger = ctx.Value(loggerKey).(*slog.Logger)
		return nil, nil
	}

	if _, err := interceptor(context.Background(), nil, info, handler); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if !sawLogger {
		t.Error("loggerKey not present in handler context")
	}
}

func TestGRPCUnaryServerInterceptor_ErrorRecordsSpanStatus(t *testing.T) {
	exporter := setupTestTracing(t)
	interceptor := GRPCUnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/posnet.Settlement/GetBatch"}

	wantErr := grpcstatus.Error(grpccodes.NotFound, "batch not found")
	handler := func(ctx context.Context, req any) (any, error) {
		return nil, wantErr
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if !errors.Is(err, wantErr) && err.Error() != wantErr.Error() {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Status.Code != codes.Error {
		t.Errorf("span status code = %v, want %v", span.Status.Code, codes.Error)
	}
	if span.Status.Description != wantErr.Error() {
		t.Errorf("span status description = %q, want %q", span.Status.Description, wantErr.Error())
	}
	if v, ok := attrValue(span.Attributes, "rpc.grpc_status"); !ok || v.AsString() != grpccodes.NotFound.String() {
		t.Errorf("rpc.grpc_status attribute = %v, ok=%v, want %q", v, ok, grpccodes.NotFound.String())
	}
}

func TestGRPCUnaryServerInterceptor_NoIncomingMetadata(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/posnet.Settlement/GetBatch"}

	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }

	// context.Background() no lleva metadata gRPC entrante — no debe hacer panic.
	if _, err := interceptor(context.Background(), nil, info, handler); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
}

func TestGRPCUnaryServerInterceptor_ExtractsIncomingTraceParent(t *testing.T) {
	exporter := setupTestTracing(t)
	interceptor := GRPCUnaryServerInterceptor()
	info := &grpc.UnaryServerInfo{FullMethod: "/posnet.Settlement/GetBatch"}

	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	md := metadata.Pairs("traceparent", "00-"+incomingTraceID+"-00f067aa0ba902b7-01")
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req any) (any, error) { return nil, nil }
	if _, err := interceptor(ctx, nil, info, handler); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if got := spans[0].SpanContext.TraceID().String(); got != incomingTraceID {
		t.Errorf("propagated trace_id = %q, want %q", got, incomingTraceID)
	}
}

// ─── GRPCUnaryClientInterceptor ───────────────────────────────────────────────

func TestGRPCUnaryClientInterceptor_Success(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryClientInterceptor()

	var invoked bool
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		invoked = true
		if method != "/posnet.Settlement/GetBatch" {
			t.Errorf("method = %q, want %q", method, "/posnet.Settlement/GetBatch")
		}
		return nil
	}

	err := interceptor(context.Background(), "/posnet.Settlement/GetBatch", "req", "reply", nil, invoker)
	if err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if !invoked {
		t.Error("invoker was not called")
	}
}

func TestGRPCUnaryClientInterceptor_InjectsOutgoingTraceContext(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryClientInterceptor()

	var gotTraceparent string
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			t.Fatal("outgoing metadata missing — expected trace context to be injected")
		}
		vals := md.Get("traceparent")
		if len(vals) > 0 {
			gotTraceparent = vals[0]
		}
		return nil
	}

	if err := interceptor(context.Background(), "/posnet.Settlement/GetBatch", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if gotTraceparent == "" {
		t.Error("traceparent header was not injected into outgoing metadata")
	}
}

func TestGRPCUnaryClientInterceptor_PreservesExistingOutgoingMetadata(t *testing.T) {
	setupTestTracing(t)
	interceptor := GRPCUnaryClientInterceptor()

	ctx := metadata.AppendToOutgoingContext(context.Background(), "x-custom-header", "custom-value")

	var gotCustomHeader string
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(ctx)
		vals := md.Get("x-custom-header")
		if len(vals) > 0 {
			gotCustomHeader = vals[0]
		}
		return nil
	}

	if err := interceptor(ctx, "/posnet.Settlement/GetBatch", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor error = %v", err)
	}
	if gotCustomHeader != "custom-value" {
		t.Errorf("x-custom-header = %q, want %q (no debe perderse al inyectar el trace context)", gotCustomHeader, "custom-value")
	}
}

func TestGRPCUnaryClientInterceptor_ErrorIsRecordedAndPropagated(t *testing.T) {
	exporter := setupTestTracing(t)
	interceptor := GRPCUnaryClientInterceptor()

	wantErr := errors.New("connection refused")
	invoker := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return wantErr
	}

	err := interceptor(context.Background(), "/posnet.Settlement/GetBatch", nil, nil, nil, invoker)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("exported spans = %d, want 1", len(spans))
	}
	if spans[0].Status.Code != codes.Error {
		t.Errorf("span status code = %v, want %v", spans[0].Status.Code, codes.Error)
	}
}

// ─── grpcMetadataCarrier ──────────────────────────────────────────────────────

func TestGrpcMetadataCarrier_Get(t *testing.T) {
	md := metadata.New(map[string]string{"traceparent": "value-1"})
	c := grpcMetadataCarrier{md: md}

	if got := c.Get("traceparent"); got != "value-1" {
		t.Errorf("Get(traceparent) = %q, want %q", got, "value-1")
	}
	if got := c.Get("missing-key"); got != "" {
		t.Errorf("Get(missing-key) = %q, want empty", got)
	}
}

func TestGrpcMetadataCarrier_Set(t *testing.T) {
	md := metadata.New(nil)
	c := grpcMetadataCarrier{md: md}
	c.Set("traceparent", "value-1")

	if got := md.Get("traceparent"); len(got) != 1 || got[0] != "value-1" {
		t.Errorf("md.Get(traceparent) = %v, want [value-1]", got)
	}
}

func TestGrpcMetadataCarrier_Keys(t *testing.T) {
	md := metadata.New(map[string]string{"traceparent": "v1", "baggage": "v2"})
	c := grpcMetadataCarrier{md: md}

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

// ─── extractGRPCTraceContext / injectGRPCTraceContext ────────────────────────

func TestExtractGRPCTraceContext_NoIncomingMetadata(t *testing.T) {
	setupTestTracing(t)
	ctx := context.Background()

	got := extractGRPCTraceContext(ctx)
	if got != ctx {
		t.Error("extractGRPCTraceContext() should return the same context unchanged when there's no incoming metadata")
	}
}

func TestInjectGRPCTraceContext_NoExistingOutgoingMetadata(t *testing.T) {
	setupTestTracing(t)
	// El propagador TraceContext solo inyecta si hay un span válido en el
	// contexto — sin uno, Inject() es un no-op silencioso.
	ctx, span := StartSpan(context.Background(), "test-span")
	defer span.End()

	got := injectGRPCTraceContext(ctx)
	md, ok := metadata.FromOutgoingContext(got)
	if !ok {
		t.Fatal("expected outgoing metadata to be created")
	}
	if len(md.Get("traceparent")) == 0 {
		t.Error("expected traceparent to be injected from the active span")
	}
}
