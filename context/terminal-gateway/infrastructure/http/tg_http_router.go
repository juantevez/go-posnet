package http

import (
	"net/http"

	"github.com/juantevez/go-posnet/context/terminal-gateway/application/port"
	"github.com/juantevez/go-posnet/context/terminal-gateway/application/query"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// NewRouter construye el mux HTTP del BC Terminal Gateway con middlewares aplicados.
func NewRouter(
	sessionService port.SessionService,
	queryHandler *query.SessionQueryHandler,
	pool pgutil.PgxPool,
) http.Handler {
	mux := http.NewServeMux()

	// Rutas existentes
	h := NewHandler(sessionService, queryHandler, pool)
	h.RegisterRoutes(mux)

	// Rutas del flujo QR — simulador POSNET
	qr := NewQRHandler(sessionService, queryHandler)
	qr.RegisterQRRoutes(mux)

	mux.Handle("GET /metrics", metricsHandler())

	// CORS antes de observability para que el preflight OPTIONS no quede bloqueado
	return corsMiddleware(observability.HTTPMiddleware(recoverMiddleware(mux)))
}

func metricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("# Prometheus metrics endpoint\n"))
	})
}

// corsMiddleware permite requests desde cualquier origen.
// Necesario para que el frontend (Live Server / puerto 5500) pueda
// llamar al backend (puerto 8081) sin ser bloqueado por el browser.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Terminal-ID")

		// El browser envía un preflight OPTIONS antes del POST real
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
