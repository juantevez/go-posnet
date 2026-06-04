package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
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
