package observability

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// TracerConfig contiene la configuración del proveedor de trazas.
type TracerConfig struct {
	ServiceName    string
	ServiceVersion string
	OTLPEndpoint   string // ej: "otel-collector:4317"
	Environment    string // "production" | "staging" | "development"
}

// InitTracer configura el TracerProvider global con exportación a OTLP.
// Retorna una función shutdown que debe llamarse al cerrar el servicio.
// Llamado una sola vez en cmd/{bc}/main.go.
func InitTracer(ctx context.Context, cfg TracerConfig) (shutdown func(context.Context) error, err error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(), // TLS manejado externamente (service mesh / mTLS)
	)
	if err != nil {
		return nil, fmt.Errorf("observability: init OTLP exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment", cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()), // Ajustar en producción con TraceIDRatio
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// StartSpan crea un span hijo del contexto actual.
// Uso:
//
//	ctx, span := observability.StartSpan(ctx, "repository.findByID")
//	defer span.End()
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer("go-posnet")
	return tracer.Start(ctx, name, opts...)
}

// RecordError anota un error en el span activo y lo marca con StatusError.
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// AddEvent agrega un evento anotado al span activo.
// Útil para marcar hitos: "emv_validated", "fraud_check_sent", etc.
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// enrichWithTrace inyecta trace_id y span_id en el logger si hay un span activo.
func enrichWithTrace(ctx context.Context, logger *slog.Logger) *slog.Logger {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return logger
	}
	return logger.With(
		slog.String("trace_id", span.SpanContext().TraceID().String()),
		slog.String("span_id", span.SpanContext().SpanID().String()),
	)
}
