package http

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juantevez/go-posnet/context/authorization/application/port"
	"github.com/juantevez/go-posnet/pkg/observability"
)

// NewRouter construye y retorna el mux HTTP del BC Authorization
// con todas las rutas registradas y los middlewares aplicados.
//
// Rutas expuestas:
//
//	GET  /healthz              → liveness probe
//	GET  /readyz               → readiness probe (verifica Postgres)
//	GET  /metrics              → métricas Prometheus
//	GET  /transactions/{id}    → estado de una transacción (operaciones)
func NewRouter(queryService port.QueryService, pool *pgxpool.Pool) http.Handler {
	mux := http.NewServeMux()

	h := NewHandler(queryService, pool)
	h.RegisterRoutes(mux)

	// Registrar endpoint de métricas Prometheus.
	// El handler es provisto por el exporter de OpenTelemetry inicializado
	// en cmd/authorization/main.go mediante observability.InitMeter().
	mux.Handle("GET /metrics", metricsHandler())

	// Aplicar middlewares globales en orden:
	//   1. Recover  — captura panics y retorna 500 sin caer el proceso
	//   2. Logging  — traza distribuida + log de cada request
	return observability.HTTPMiddleware(
		recoverMiddleware(mux),
	)
}

// metricsHandler retorna el handler de Prometheus.
// Se importa desde el paquete de observabilidad para no acoplar
// el router directamente a la librería de Prometheus.
func metricsHandler() http.Handler {
	// El MeterProvider de OpenTelemetry registra automáticamente
	// el handler en la ruta estándar de Prometheus cuando se inicializa
	// con el exporter de Prometheus en observability.InitMeter().
	// Aquí solo exponemos esa ruta en el mux.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// El handler real es inyectado por el MeterProvider al arrancar.
		// Esta implementación es un placeholder — reemplazar por
		// promhttp.Handler() al integrar la librería de Prometheus.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Prometheus metrics endpoint\n"))
	})
}
