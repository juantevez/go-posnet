package command

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/juantevez/go-posnet/pkg/observability"
)

// Metrics agrupa los instrumentos de negocio del BC Notification.
//
// Nombre final en Prometheus (el exporter agrega _total a counters y _seconds
// a histogramas con unidad "s"):
//
//	posnet_notifications_sent    → posnet_notifications_sent_total{channel}
//	posnet_notifications_failed  → posnet_notifications_failed_total{channel}
//	posnet_grpc_request_duration → posnet_grpc_request_duration_seconds{channel}
//
// El label "bc" lo inyecta Prometheus como target label (prometheus.yml).
type Metrics struct {
	sent     metric.Int64Counter
	failed   metric.Int64Counter
	duration metric.Float64Histogram
}

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}

// NewMetrics crea los instrumentos usando el MeterProvider global.
func NewMetrics() (*Metrics, error) {
	m := observability.Meter("posnet.notification")

	sent, err := m.Int64Counter("posnet_notifications_sent",
		metric.WithDescription("Notificaciones entregadas, desglosadas por channel."))
	if err != nil {
		return nil, err
	}
	failed, err := m.Int64Counter("posnet_notifications_failed",
		metric.WithDescription("Fallos de entrega, desglosados por channel."))
	if err != nil {
		return nil, err
	}
	duration, err := m.Float64Histogram("posnet_grpc_request_duration",
		metric.WithDescription("Latencia de la entrega outbound (gRPC/webhook) por channel."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(latencyBuckets...))
	if err != nil {
		return nil, err
	}

	return &Metrics{sent: sent, failed: failed, duration: duration}, nil
}

// RecordDelivery contabiliza un intento de entrega (sent o failed) y su latencia,
// por channel ("terminal" o "webhook"). Nil-safe.
func (m *Metrics) RecordDelivery(ctx context.Context, channel string, delivered bool, d time.Duration) {
	if m == nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("channel", channel))
	if delivered {
		m.sent.Add(ctx, 1, attrs)
	} else {
		m.failed.Add(ctx, 1, attrs)
	}
	m.duration.Record(ctx, d.Seconds(), attrs)
}
