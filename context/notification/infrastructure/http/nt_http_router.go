package http

import (
	"net/http"

	"github.com/juantevez/go-posnet/context/notification/application/command"
	"github.com/juantevez/go-posnet/context/notification/application/query"
	"github.com/juantevez/go-posnet/pkg/observability"
	"github.com/juantevez/go-posnet/pkg/pgutil"
)

// NewRouter construye el mux HTTP del BC Notification con middlewares aplicados.
func NewRouter(
	queryHandler *query.NotificationQueryHandler,
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
