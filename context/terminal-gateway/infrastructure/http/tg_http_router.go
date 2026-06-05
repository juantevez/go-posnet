package http

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tu-org/posnet-backend/context/terminal-gateway/application/port"
	"github.com/tu-org/posnet-backend/context/terminal-gateway/application/query"
	"github.com/tu-org/posnet-backend/pkg/observability"
)

// NewRouter construye el mux HTTP del BC Terminal Gateway con middlewares aplicados.
//
// Rutas expuestas:
//
//	GET  /healthz                  → liveness probe
//	GET  /readyz                   → readiness probe
//	GET  /metrics                  → métricas Prometheus
//	GET  /sessions/{id}            → estado de sesión (operaciones)
//	POST /sessions/{id}/cancel     → cancelación manual
//	POST /sessions/{id}/reversal   → solicitud de anulación
//	POST /batch-close              → cierre de lote
//
// NOTA: las conexiones WebSocket de los terminales se sirven en un puerto
// separado (WSPort) gestionado por infrastructure/websocket/ — no aquí.
func NewRouter(
	sessionService port.SessionService,
	queryHandler *query.SessionQueryHandler,
	pool *pgxpool.Pool,
) http.Handler {
	mux := http.NewServeMux()

	h := NewHandler(sessionService, queryHandler, pool)
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
