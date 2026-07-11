package command

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/juantevez/go-posnet/pkg/observability"
)

// Metrics agrupa los instrumentos de negocio del BC Terminal Gateway.
//
// Nombre final en Prometheus (el exporter agrega _total a counters y _seconds
// a histogramas con unidad "s"):
//
//	posnet_sessions_created  → posnet_sessions_created_total
//	posnet_sessions_approved → posnet_sessions_approved_total
//	posnet_sessions_expired  → posnet_sessions_expired_total
//	posnet_qr_e2e_duration   → posnet_qr_e2e_duration_seconds
//
// El label "bc" lo inyecta Prometheus como target label (prometheus.yml).
type Metrics struct {
	created  metric.Int64Counter
	approved metric.Int64Counter
	expired  metric.Int64Counter
	qrE2E    metric.Float64Histogram
}

// e2eBuckets cubre la latencia E2E del flujo QR (~1s p50, saga completa).
var e2eBuckets = []float64{0.1, 0.25, 0.5, 1, 1.5, 2, 3, 5, 10, 30}

// NewMetrics crea los instrumentos usando el MeterProvider global.
func NewMetrics() (*Metrics, error) {
	m := observability.Meter("posnet.terminal_gateway")

	created, err := m.Int64Counter("posnet_sessions_created",
		metric.WithDescription("Sesiones QR creadas."))
	if err != nil {
		return nil, err
	}
	approved, err := m.Int64Counter("posnet_sessions_approved",
		metric.WithDescription("Sesiones que llegaron a APPROVED."))
	if err != nil {
		return nil, err
	}
	expired, err := m.Int64Counter("posnet_sessions_expired",
		metric.WithDescription("Sesiones vencidas sin pago (TTL)."))
	if err != nil {
		return nil, err
	}
	qrE2E, err := m.Float64Histogram("posnet_qr_e2e_duration",
		metric.WithDescription("Latencia E2E del flujo QR desde created hasta APPROVED."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(e2eBuckets...))
	if err != nil {
		return nil, err
	}

	return &Metrics{
		created:  created,
		approved: approved,
		expired:  expired,
		qrE2E:    qrE2E,
	}, nil
}

func (m *Metrics) RecordCreated(ctx context.Context) {
	if m == nil {
		return
	}
	m.created.Add(ctx, 1)
}

// RecordApproved contabiliza una sesión aprobada y su latencia E2E desde created.
func (m *Metrics) RecordApproved(ctx context.Context, createdAt time.Time) {
	if m == nil {
		return
	}
	m.approved.Add(ctx, 1)
	m.qrE2E.Record(ctx, time.Since(createdAt).Seconds())
}

// RecordExpired contabiliza n sesiones vencidas (batch del reaper). Nil-safe.
func (m *Metrics) RecordExpired(ctx context.Context, n int64) {
	if m == nil || n <= 0 {
		return
	}
	m.expired.Add(ctx, n)
}
