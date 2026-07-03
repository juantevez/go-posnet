package http

import (
	"net/http"

	"github.com/juantevez/go-posnet/context/fraud-detection/application/command"
	"github.com/juantevez/go-posnet/context/fraud-detection/application/query"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// NewRouter construye el mux HTTP del BC Fraud Detection con middlewares aplicados.
//
// Rutas expuestas:
//
//	GET /healthz                           → liveness probe
//	GET /readyz                            → readiness probe
//	GET /metrics                           → métricas Prometheus
//	GET /fraud-cases/{transaction_id}      → resultado del análisis de una transacción
//	GET /rules                             → lista reglas activas con umbrales
//	PUT /rules/{rule_id}/threshold         → actualiza umbral de una regla (admin)
func NewRouter(
	queryHandler *query.FraudQueryHandler,
	adminHandler *command.AdminHandler,
	pool pgutil.PgxPool,
) http.Handler {
	mux := http.NewServeMux()

	h := NewHandler(queryHandler, adminHandler, pool)
	h.RegisterRoutes(mux)

	mux.Handle("GET /metrics", metricsHandler())

	return observability.HTTPMiddleware(recoverMiddleware(mux))
}

func metricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Prometheus metrics endpoint\n"))
	})
}
