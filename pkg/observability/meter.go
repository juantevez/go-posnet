package observability

import (
	"context"
	"fmt"
	"sync"

	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// InitMeter configura el MeterProvider global con exportación a Prometheus.
// Llamado una sola vez en cmd/{bc}/main.go junto con InitTracer.
// El exporter de Prometheus expone las métricas en el endpoint /metrics
// del servidor HTTP de cada BC.
func InitMeter(ctx context.Context, serviceName string) (shutdown func(context.Context) error, err error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("observability: init prometheus exporter: %w", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(exporter),
	)

	otel.SetMeterProvider(provider)

	return provider.Shutdown, nil
}

// Meter retorna un Meter con el nombre del instrumento dado.
// Uso: meter := observability.Meter("posnet.authorization")
func Meter(name string) metric.Meter {
	return otel.GetMeterProvider().Meter(name)
}

var (
	natsProcessedOnce sync.Once
	natsProcessed     metric.Int64Counter
)

// RecordNATSProcessed contabiliza un mensaje NATS entregado a un subscriber,
// desglosado por subject. Nombre final: posnet_nats_messages_processed_total.
// El label "bc" lo agrega Prometheus como target label. Nil-safe y lazy: se
// inicializa en la primera llamada (tras InitMeter).
func RecordNATSProcessed(ctx context.Context, subject string) {
	natsProcessedOnce.Do(func() {
		natsProcessed, _ = Meter("posnet.nats").Int64Counter(
			"posnet_nats_messages_processed",
			metric.WithDescription("Mensajes NATS entregados a los subscribers, por subject."),
		)
	})
	if natsProcessed != nil {
		natsProcessed.Add(ctx, 1, metric.WithAttributes(attribute.String("subject", subject)))
	}
}

var (
	natsFailedOnce sync.Once
	natsFailed     metric.Int64Counter
)

// RecordNATSFailed contabiliza un mensaje NATS que falló su procesamiento,
// desglosado por subject y kind ("validation" = error permanente → Term,
// "transient" = error transitorio → Nak, "panic"). Nombre final:
// posnet_nats_messages_failed_total. El label "bc" lo agrega Prometheus.
func RecordNATSFailed(ctx context.Context, subject, kind string) {
	natsFailedOnce.Do(func() {
		natsFailed, _ = Meter("posnet.nats").Int64Counter(
			"posnet_nats_messages_failed",
			metric.WithDescription("Mensajes NATS que fallaron su procesamiento, por subject y kind."),
		)
	})
	if natsFailed != nil {
		natsFailed.Add(ctx, 1, metric.WithAttributes(
			attribute.String("subject", subject),
			attribute.String("kind", kind),
		))
	}
}

// MetricsHandler retorna el handler HTTP que expone las métricas en formato
// Prometheus para el endpoint /metrics de cada BC.
//
// Sirve el registry default de Prometheus, que incluye:
//   - Las métricas de negocio de OpenTelemetry (el exporter creado en
//     InitMeter se registra en el registry default).
//   - Las métricas base del runtime Go y del proceso (go_goroutines,
//     go_memstats_*, process_cpu_seconds_total, ...), auto-registradas
//     por client_golang.
//
// InitMeter debe haberse llamado antes para que las métricas de negocio
// aparezcan; las métricas Go base están disponibles sin InitMeter.
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// ─── Métricas estándar del sistema ───────────────────────────────────────────
//
// Cada BC instancia las métricas que necesita usando Meter().
// Las siguientes son las métricas globales del sistema POSNET definidas
// en el documento de arquitectura. Cada BC crea solo las que le corresponden.
//
// Nombres de métricas (Prometheus):
//
//   posnet_transactions_total{state="approved|rejected", bc="authorization"}
//     Counter — total de transacciones por estado final.
//
//   posnet_authorization_duration_seconds{quantile="0.5|0.95|0.99"}
//     Histogram — latencia E2E del ciclo de autorización.
//
//   posnet_acquirer_request_duration_seconds{quantile="0.5|0.95|0.99"}
//     Histogram — latencia de la llamada al host adquirente externo.
//
//   posnet_fraud_score{le="..."}
//     Histogram — distribución de scores de fraude calculados.
//
//   posnet_nats_consumer_lag{consumer="auth-txn-receiver|..."}
//     Gauge — mensajes pendientes en cada durable consumer.
//     Alerta si supera 100.
//
//   posnet_active_sessions
//     Gauge — sesiones WebSocket activas en Terminal Gateway.
//
//   posnet_webhook_delivery_failures_total{merchant_id="..."}
//     Counter — fallos de entrega de webhook por comercio.
//
//   posnet_batch_close_duration_seconds{terminal_id="..."}
//     Histogram — duración del cierre de lote por terminal.
