package observability

import (
	"context"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
)

// natsCarrier adapta nats.Header para que sea compatible con
// la interfaz TextMapCarrier de OpenTelemetry.
type natsCarrier struct{ header nats.Header }

func (c natsCarrier) Get(key string) string { return c.header.Get(key) }
func (c natsCarrier) Set(key, val string)   { c.header.Set(key, val) }
func (c natsCarrier) Keys() []string {
	keys := make([]string, 0, len(c.header))
	for k := range c.header {
		keys = append(keys, k)
	}
	return keys
}

// InjectTraceContext inyecta el trace context del ctx en los headers del mensaje NATS.
// Llamado por pkg/natsutil/publisher.go antes de cada Publish.
// Propaga TraceID y SpanID para que el consumer pueda continuar la traza.
func InjectTraceContext(ctx context.Context, msg *nats.Msg) {
	if msg.Header == nil {
		msg.Header = make(nats.Header)
	}
	otel.GetTextMapPropagator().Inject(ctx, natsCarrier{header: msg.Header})
}

// ExtractTraceContext extrae el trace context de los headers del mensaje NATS
// y retorna un contexto Go con el span padre reconstruido.
// Llamado por el subscriber al recibir un mensaje.
func ExtractTraceContext(ctx context.Context, msg *nats.Msg) context.Context {
	if msg.Header == nil {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, natsCarrier{header: msg.Header})
}
